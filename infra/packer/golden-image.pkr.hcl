# Golden VM image: Debian 12 cloud image -> Ansible hardening (devsec.hardening
# + custom firewall/auditd/Wazuh-agent roles) -> Lynis gate. This is the same
# "rebuild, don't patch in place" pattern as docker/base/ (Epic 2) applied to
# VMs instead of containers - infra/terraform-state/'s Postgres host and
# infra/nexus/'s host (Epic 2/4) are both explicitly meant to move onto this
# image once it exists, instead of running on whatever unhardened box they're
# on today.
#
# Uses the `qemu` builder against a public Debian cloud qcow2 - produces a
# qcow2 any self-hosted hypervisor can boot, deliberately not a cloud-
# provider-specific image (AMI/GCE image), consistent with staying
# provider-agnostic.
#
# NOT YET WIRED TO ANYTHING: there's no Terraform module that provisions a
# VM from this image's output - Epic 4's module only covers the k8s cluster
# (local kind). Consuming this image to actually stand up a VM is unnamed
# future work, not part of this epic. This epic's scope is: produce a
# hardened, versioned image and gate its quality - using it is later.

packer {
  required_plugins {
    qemu = {
      source  = "github.com/hashicorp/qemu"
      version = "~> 1.1"
    }
  }
}

locals {
  output_dir = "output/golden-${var.image_version}"
}

source "qemu" "golden" {
  iso_url      = "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2"
  iso_checksum = "file:https://cloud.debian.org/images/cloud/bookworm/latest/SHA512SUMS"

  disk_image       = true
  format           = "qcow2"
  disk_size        = "10240M"
  accelerator      = "kvm"
  headless         = true
  memory           = 2048
  output_directory = local.output_dir
  vm_name          = "golden-${var.image_version}.qcow2"

  ssh_username         = "debian"
  ssh_private_key_file = var.ssh_private_key_file
  ssh_timeout          = "10m"

  # NoCloud cloud-init seed, inline rather than static files so the real
  # SSH public key can be templated in at build time without a separate
  # pre-build script.
  cd_label = "cidata"
  cd_content = {
    "meta-data" = ""
    "user-data" = templatefile("${path.root}/cloud-init/user-data.pkrtpl", {
      ssh_public_key = var.ssh_public_key
    })
  }

  shutdown_command = "sudo -S shutdown -P now"
}

build {
  sources = ["source.qemu.golden"]

  provisioner "ansible" {
    playbook_file = "${path.root}/ansible/playbook.yml"
    extra_arguments = [
      "--extra-vars", "wazuh_manager_address=${var.wazuh_manager_address}",
    ]
  }

  provisioner "shell" {
    script = "${path.root}/scripts/lynis-gate.sh"
  }

  post-processor "manifest" {
    output     = "manifest.json"
    strip_path = true
    custom_data = {
      image_version = var.image_version
    }
  }
}
