# Golden VM image

CIS-aligned hardening baseline for every VM in this project that isn't a container
(the standalone Postgres host for `infra/terraform-state/`, the Nexus host for
`infra/nexus/`, the eventual Wazuh manager in Epic 7, the VPN host in Epic 8) - the
same "rebuild, don't patch in place" versioning idea as `docker/base/` (Epic 2),
applied to VMs.

## Pieces

- `golden-image.pkr.hcl`, `variables.pkr.hcl` - Packer template, `qemu` builder
  against Debian 12's public cloud image.
- `cloud-init/user-data.pkrtpl` - NoCloud seed, templated with the real SSH public
  key at build time so no key material is ever committed.
- `ansible/` - hardening: `devsec.hardening.os_hardening` + `.ssh_hardening` (the
  maintained, independently-written CIS/STIG-inspired collection - not a
  reproduction of CIS's own copyrighted benchmark), plus custom roles for
  `firewall` (ufw, default-deny inbound), `auditd` (baseline audit rules - see that
  role's caveat on scope), and `wazuh_agent` (installs + enables, not yet
  enrollable - see its caveat).
- `scripts/lynis-gate.sh` - fails the build if the Lynis hardening index is below
  `LYNIS_MIN_SCORE` (default 70, set via env in `.github/workflows/golden-image.yml`).

## Building

```
ssh-keygen -t ed25519 -f /tmp/packer_key -N ""
cd infra/packer
packer init golden-image.pkr.hcl
packer build \
  -var "ssh_public_key=$(cat /tmp/packer_key.pub)" \
  -var "ssh_private_key_file=/tmp/packer_key" \
  -var "image_version=$(git rev-parse --short HEAD)" \
  golden-image.pkr.hcl
```

Needs `/dev/kvm` locally (or a CI runner that has it - see `golden-image.yml`'s
header comment on that caveat).

## What this epic does NOT do

- **Doesn't provision any actual VM from this image.** There's no Terraform module
  that consumes the output qcow2/manifest yet - Epic 4's `modules/k8s-cluster`
  interface is specifically for the k8s cluster, not generic VM hosts. Using this
  image to stand up `infra/terraform-state/`'s Postgres host (etc.) for real is
  unnamed future work.
- **Doesn't decide where the built image is stored long-term.** Currently a 90-day
  GitHub Actions artifact - fine for now, not durable. Nexus's eventual "raw" hosted
  repo (once Epic 2's Nexus is actually deployed, which itself needs Epic 4's
  cluster) is a candidate, not a decision.
- **Doesn't claim full CIS benchmark compliance.** `devsec.hardening` is
  CIS/STIG-inspired, not a verbatim implementation, and the audit rules in
  `ansible/roles/auditd/files/cis-audit.rules` are a hand-written minimal starting
  set, not a transcription of CIS's copyrighted rule text. Lynis's score is a
  reasonable proxy for "is this hardened," not a CIS compliance certificate.
