# cert-manager + internal CA

Resolves the TLS decision Traefik (Epic 4) and Vault (Epic 6) both deferred -
see `cluster-issuers.yaml`'s header for why an internal CA, not public ACME.

## Order of operations

1. Install the `cert-manager` chart.
2. `kubectl apply -f cluster-issuers.yaml`
3. Extract the root CA cert and distribute it to every admin's VPN-connected device,
   so their browser/OS trusts certs issued by `cerberus-internal-ca`:
   ```
   kubectl get secret cerberus-internal-ca-secret -n cert-manager -o jsonpath='{.data["tls.crt"]}' | base64 -d > cerberus-ca.crt
   ```
   Import `cerberus-ca.crt` into each admin's OS/browser trust store. This is a
   manual, per-device step, same "a human has to do this, documented not automated"
   pattern as Vault's unseal - there's no way to push trust into someone's OS from
   inside this repo.
4. Any Ingress/Certificate referencing `issuerRef: {name: cerberus-internal-ca, kind:
   ClusterIssuer}` now gets a real, auto-renewed cert - see
   `infra/k8s/oauth2-proxy/ingressroutes.yaml` and `infra/k8s/vault/certificate.yaml`.

## Not yet deployable

Needs Epic 4's cluster.
