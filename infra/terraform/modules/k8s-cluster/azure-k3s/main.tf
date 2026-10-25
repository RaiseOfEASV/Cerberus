# Azure implementation of the k8s-cluster module interface (see
# ../README.md) - self-hosted k3s on a single VM, not AKS. Matches this
# project's self-hosted-everything theme (own runners, own Vault, own
# Nexus, own VPN) over a managed control plane. Single node for now:
# control-plane + worker combined, which k3s supports out of the box
# (unlike kubeadm, it doesn't taint server nodes, so workloads schedule
# here fine). Multi-node is a known gap - see node_count's validation.

terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
    null = {
      source  = "hashicorp/null"
      version = "~> 3.0"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.0"
    }
  }
}

# Reads the resource group + VNet infra/terraform/bootstrap/ already
# created - this module does NOT own or create them. Two separate
# Terraform roots can't both manage the same resource, so everything
# here is a read-only lookup plus new resources (subnet, NSG, VM, ...)
# layered into that existing network.
data "azurerm_resource_group" "main" {
  name = var.resource_group_name
}

data "azurerm_virtual_network" "main" {
  name                = "cerberus-vnet"
  resource_group_name = data.azurerm_resource_group.main.name
}

# New subnet inside the existing VNet - puts the cluster VM on the same
# private network as terraform-state's Postgres and the backups MinIO
# (both on 10.0.1.4), so in-cluster workloads (Vault's backup CronJob,
# Velero) can reach them over private IPs later instead of needing the
# public IP/internet or routing internal traffic through headscale.
resource "azurerm_subnet" "cluster" {
  name                 = "cerberus-cluster-subnet"
  resource_group_name  = data.azurerm_resource_group.main.name
  virtual_network_name = data.azurerm_virtual_network.main.name
  address_prefixes     = ["10.0.2.0/24"]
}

resource "azurerm_network_security_group" "cluster" {
  name                = "cerberus-cluster-nsg"
  resource_group_name = data.azurerm_resource_group.main.name
  location            = var.location

  security_rule {
    name                       = "AllowSSHAdmin"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = var.admin_source_ip
    destination_address_prefix = "*"
  }

  # Kubernetes API - restricted to the admin IP for now, same as SSH.
  # Not VPN-gated yet because kubectl needs to work before this VM has
  # joined headscale's tailnet as a client, which is a follow-up step
  # this module doesn't perform - see the module README.
  security_rule {
    name                       = "AllowKubernetesAPI"
    priority                   = 110
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "6443"
    source_address_prefix      = var.admin_source_ip
    destination_address_prefix = "*"
  }

  # Everything else (MetalLB/Traefik's future LoadBalancer IP, etc.) is
  # deliberately not opened here - that's later Epic 4 work, once there's
  # an actual ingress to expose and a considered call on public vs.
  # VPN-only.
}

resource "azurerm_public_ip" "cluster" {
  name                = "cerberus-cluster-pip"
  resource_group_name = data.azurerm_resource_group.main.name
  location            = var.location
  allocation_method   = "Static"
  sku                 = "Standard"
}

resource "azurerm_network_interface" "cluster" {
  name                = "cerberus-cluster-nic"
  resource_group_name = data.azurerm_resource_group.main.name
  location            = var.location

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.cluster.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.cluster.id
  }
}

resource "azurerm_network_interface_security_group_association" "cluster" {
  network_interface_id      = azurerm_network_interface.cluster.id
  network_security_group_id = azurerm_network_security_group.cluster.id
}

resource "azurerm_linux_virtual_machine" "cluster" {
  name                = "${var.cluster_name}-k3s-vm"
  resource_group_name = data.azurerm_resource_group.main.name
  location            = var.location
  size                = var.vm_size
  admin_username      = var.admin_username

  network_interface_ids = [azurerm_network_interface.cluster.id]

  admin_ssh_key {
    username   = var.admin_username
    public_key = file(pathexpand(var.ssh_public_key_path))
  }

  disable_password_authentication = true

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts-gen2"
    version   = "latest"
  }

  # k3s installs itself via cloud-init, unlike the bootstrap VM's Docker
  # Compose stacks (which needed a manual follow-up step because they
  # involve secrets that must never land in cloud-init or Terraform
  # state). Standing up k3s itself has no secret material - the node
  # join token it generates only matters once a second node needs to
  # join, which isn't implemented yet (see node_count's validation).
  #
  # --tls-san matters: k3s's self-signed API server cert only covers the
  # SANs it knows about at install time (private IP, cluster IP,
  # localhost) by default - hit this for real as a cert verification
  # error from kubectl ("certificate is valid for 10.0.2.4, ... not
  # <public ip>") the first time this was applied. Without this flag the
  # kubeconfig this module fetches (pointed at the public IP, since
  # that's the only address reachable from outside the VM) would never
  # actually work.
  custom_data = base64encode(<<-CLOUDINIT
    #cloud-config
    runcmd:
      - curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION=${var.kubernetes_version} sh -s - server --write-kubeconfig-mode 644 --tls-san ${azurerm_public_ip.cluster.ip_address}
    CLOUDINIT
  )
}

# Pulls the kubeconfig off the VM once k3s is up, rewriting its
# 127.0.0.1 server URL to the VM's real public IP. kind's provider hands
# back a ready-to-use kubeconfig directly as a resource attribute; k3s
# instead writes one to disk on the node that only works from localhost,
# so this module has to fetch + rewrite it to honor the same output
# contract. Retried in a loop because cloud-init's runcmd finishing
# doesn't guarantee k3s has written the file yet.
resource "null_resource" "fetch_kubeconfig" {
  depends_on = [azurerm_linux_virtual_machine.cluster]

  connection {
    type        = "ssh"
    host        = azurerm_public_ip.cluster.ip_address
    user        = var.admin_username
    private_key = file(pathexpand(var.ssh_private_key_path))
    timeout     = "5m"
  }

  provisioner "remote-exec" {
    inline = [
      "for i in $(seq 1 30); do [ -f /etc/rancher/k3s/k3s.yaml ] && break; sleep 10; done",
      "sudo sed \"s/127.0.0.1/${azurerm_public_ip.cluster.ip_address}/\" /etc/rancher/k3s/k3s.yaml > /tmp/kubeconfig-fetched.yaml"
    ]
  }

  provisioner "local-exec" {
    command = "scp -o StrictHostKeyChecking=accept-new -i ${pathexpand(var.ssh_private_key_path)} ${var.admin_username}@${azurerm_public_ip.cluster.ip_address}:/tmp/kubeconfig-fetched.yaml ${path.module}/kubeconfig-${var.cluster_name}.yaml"
  }
}

# Gitignored via the repo's existing `kubeconfig*.yaml` rule - this is a
# real credential, same as any other kubeconfig.
data "local_file" "kubeconfig" {
  depends_on = [null_resource.fetch_kubeconfig]
  filename   = "${path.module}/kubeconfig-${var.cluster_name}.yaml"
}
