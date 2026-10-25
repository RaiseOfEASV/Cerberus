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
    primary                       = true
    subnet_id                     = azurerm_subnet.cluster.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.cluster.id
  }

  # MetalLB's default L2/ARP mode doesn't work on Azure's SDN unmodified -
  # Azure only routes traffic to IPs it actually knows about on a NIC, it
  # won't deliver to an address a VM just ARPs for on its own. Registering
  # this IP as a real secondary IP configuration here is what makes
  # Azure's network actually forward traffic to it; MetalLB's
  # L2Advertisement (infra/k8s/metallb/ip-address-pool.yaml) then answers
  # ARP for it as usual on top of that. Single IP, not a range/pool -
  # this design only ever needs one (Traefik's own Service), see
  # infra/headscale/config/config.yaml's own comment on why every admin
  # hostname resolves to the same IP.
  ip_configuration {
    name                          = "metallb-pool"
    subnet_id                     = azurerm_subnet.cluster.id
    private_ip_address_allocation = "Static"
    private_ip_address            = var.metallb_pool_ip
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
  #
  # --disable traefik/servicelb: k3s bundles its own Traefik + ServiceLB
  # (klipper) by default, which would otherwise fight the plan's own
  # Helm-installed MetalLB + Traefik (infra/k8s/README.md) over the
  # LoadBalancer IP. Disabled so only the deliberately-configured stack
  # runs.
  #
  # --node-ip matters too: Azure's Linux agent brings up the NIC's
  # secondary IP config (the MetalLB pool address) as eth0's OS-level
  # primary address, ahead of the "internal" config this module marks
  # `primary = true` at the Azure resource level - confirmed by actually
  # SSHing in and checking `ip addr`. Without this flag k3s auto-detects
  # its own node IP as the MetalLB pool address, which then collides
  # with MetalLB's L2Advertisement trying to claim that same address as
  # a floating Service VIP. Pinning it to the NIC's real primary address
  # keeps the node's identity and the floating VIP separate.
  #
  # pod-egress-snat-fix matters for a related reason, caught by actually
  # exec-ing into a pod and testing outbound internet access: with two
  # private IPs on eth0, flannel's generic MASQUERADE rule for pod-CIDR
  # egress nondeterministically picks the MetalLB pool address (no
  # public IP attached) instead of this node's real primary address (the
  # one Azure's outbound NAT actually maps to the public IP) - confirmed
  # by tcpdump on eth0: SYNs sourced from the pool address never got a
  # SYN-ACK back, SYNs sourced from the primary address did. Fixed with
  # an explicit SNAT rule for the pod subnet, inserted ahead of flannel's
  # own rule, as a systemd service (not just a one-off runcmd command) so
  # it's reasserted on every boot, not just the first one.
  custom_data = base64encode(<<-CLOUDINIT
    #cloud-config
    write_files:
      - path: /usr/local/bin/pod-egress-snat-fix.sh
        permissions: "0755"
        content: |
          #!/bin/sh
          set -eu
          IFACE=eth0
          POD_CIDR=10.42.0.0/16
          SRC_IP=${azurerm_network_interface.cluster.private_ip_address}
          # Wait for flannel's own chain to exist first - inserting our
          # rule before flannel has set up its own would risk flannel's
          # setup landing ABOVE ours afterward (both use `-I ... 1`, so
          # whichever runs last wins the top position). Running after
          # flannel is already in place means our insert is the one that
          # ends up on top, deterministically.
          for i in $(seq 1 30); do
            iptables -t nat -L FLANNEL-POSTRTG >/dev/null 2>&1 && break
            sleep 2
          done
          if ! iptables -t nat -C POSTROUTING -s "$POD_CIDR" ! -d "$POD_CIDR" -o "$IFACE" -j SNAT --to-source "$SRC_IP" 2>/dev/null; then
            iptables -t nat -I POSTROUTING 1 -s "$POD_CIDR" ! -d "$POD_CIDR" -o "$IFACE" -j SNAT --to-source "$SRC_IP"
          fi
      - path: /etc/systemd/system/pod-egress-snat-fix.service
        permissions: "0644"
        content: |
          [Unit]
          Description=Pin pod-subnet egress SNAT to the NIC's public-IP-backed address
          After=network-online.target k3s.service
          Wants=network-online.target

          [Service]
          Type=oneshot
          ExecStart=/usr/local/bin/pod-egress-snat-fix.sh
          RemainAfterExit=true

          [Install]
          WantedBy=multi-user.target
    runcmd:
      - curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION=${var.kubernetes_version} sh -s - server --write-kubeconfig-mode 644 --tls-san ${azurerm_public_ip.cluster.ip_address} --disable traefik --disable servicelb --node-ip ${azurerm_network_interface.cluster.private_ip_address}
      - systemctl daemon-reload
      - systemctl enable --now pod-egress-snat-fix.service
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
