terraform {
  required_version = ">= 1.6.3"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
  # Deliberately LOCAL state, not the `pg` backend every other Terraform
  # root here uses - this root creates the VM that backend will live on,
  # so it can't depend on it existing yet. Same bootstrapping-paradox
  # resolution as infra/terraform-state/'s own Postgres (not managed by
  # the Terraform it backs). See README.md.
}

# Relies on `az login`'s own session for auth (no subscription_id set) -
# correct as long as exactly one subscription is active, which
# `az account show` confirmed (Azure for Students, isDefault: true).
provider "azurerm" {
  features {}
}

resource "azurerm_resource_group" "main" {
  name     = var.resource_group_name
  location = var.location
}

resource "azurerm_virtual_network" "main" {
  name                = "cerberus-vnet"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  address_space       = ["10.0.0.0/16"]
}

resource "azurerm_subnet" "main" {
  name                 = "cerberus-subnet"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.0.1.0/24"]
}

# SSH is admin-IP-restricted, not VPN-gated - this host IS the VPN, so
# there's nothing to gate behind yet when it's first provisioned. Once
# headscale is up and the admin's own device has joined it, this rule
# could be tightened further (or dropped in favor of SSH only over the
# VPN overlay) - not done automatically since that's a one-way door this
# root shouldn't take without a human confirming VPN access actually
# works first.
resource "azurerm_network_security_group" "bootstrap" {
  name                = "cerberus-bootstrap-nsg"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location

  security_rule {
    name                       = "AllowSSHAdmin"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefixes    = var.admin_source_ips
    destination_address_prefix = "*"
  }

  # headscale's coordination port - inherently public, see
  # infra/headscale/docker-compose.yml's own header on why.
  security_rule {
    name                       = "AllowHeadscaleAPI"
    priority                   = 110
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "8080"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }

  # Despite the name, this isn't headscale's - headscale only listens on
  # 8080. It's the WireGuard port of this host's own tailscaled (the VNet
  # subnet router), so admin devices can connect to it directly instead of
  # relaying through DERP. Name left as-is to avoid churning a live rule.
  security_rule {
    name                       = "AllowHeadscaleWireguard"
    priority                   = 111
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Udp"
    source_port_range          = "*"
    destination_port_range     = "41641"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }

  # Everything else (Postgres 5432, MinIO S3 API 9000) is intentionally
  # NOT opened here. Azure NSGs deny all other inbound traffic by
  # default. Those become reachable once a client joins the headscale
  # VPN - that traffic terminates locally on this host's tun interface,
  # not through the NSG's public-NIC filtering, so no extra rule is
  # needed for it to work once the VPN is live.
}

resource "azurerm_public_ip" "bootstrap" {
  name                = "cerberus-bootstrap-pip"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  allocation_method   = "Static"
  sku                 = "Standard"
}

resource "azurerm_network_interface" "bootstrap" {
  name                = "cerberus-bootstrap-nic"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.main.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.bootstrap.id
  }
}

resource "azurerm_network_interface_security_group_association" "bootstrap" {
  network_interface_id      = azurerm_network_interface.bootstrap.id
  network_security_group_id = azurerm_network_security_group.bootstrap.id
}

# Hosts headscale, terraform-state's Postgres, and the backup MinIO - all
# three standalone docker-compose stacks that are light enough to share
# one host (unlike Wazuh, which needed real isolation - deferred for now,
# see BACKLOG.local.md). Deliberately does NOT run the `docker compose up`
# for any of those stacks itself - this root's job ends at "a VM exists
# with Docker installed and SSH access works." Deploying the actual
# stacks (and their secrets, which must never land in cloud-init or
# Terraform state) is a manual follow-up step - see README.md.
resource "azurerm_linux_virtual_machine" "bootstrap" {
  name                = "cerberus-bootstrap-vm"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  size                = var.vm_size
  admin_username      = var.admin_username

  network_interface_ids = [azurerm_network_interface.bootstrap.id]

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

  custom_data = base64encode(<<-CLOUDINIT
    #cloud-config
    package_update: true
    packages:
      - docker.io
      - docker-compose-v2
    runcmd:
      - systemctl enable --now docker
      - usermod -aG docker ${var.admin_username}
    CLOUDINIT
  )
}
