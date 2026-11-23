variable "ssh_public_key" {
  type        = string
  description = "Injected into the build VM's cloud-init so Packer can SSH in. Generate ad hoc, never commit: ssh-keygen -t ed25519 -f /tmp/packer_key -N ''"
}

variable "ssh_private_key_file" {
  type        = string
  description = "Path to the private key matching ssh_public_key"
}

variable "image_version" {
  type        = string
  default     = "dev"
  description = "Tags the output image/manifest - CI passes the git SHA. Same 'rebuild, don't patch in place' versioning pattern as docker/base/ (Epic 2)."
}

