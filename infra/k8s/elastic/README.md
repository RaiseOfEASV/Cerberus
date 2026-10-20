# Elastic (ECK) - general application/infra logs

Separate from Wazuh (`infra/wazuh/`) by design - see that directory's header comment
on why security logging stays apart from the general observability stack.

## Node prerequisite

Elasticsearch normally needs `vm.max_map_count >= 262144` on the host, usually set via
a privileged initContainer. Avoided here instead by setting `node.store.allow_mmap:
false` in `elasticsearch.yaml` - a small indexing-performance tradeoff, not a
privileged container, consistent with keeping namespaces under `restricted` wherever
avoidable. If performance ever requires mmap, set the sysctl at the **node** level
(e.g. via the Epic 5 golden image, once an actual node is built from it) instead of
reaching for a privileged initContainer.

## Order of operations

1. Install the `eck-operator` chart (cluster-wide, once).
2. `kubectl apply -f elasticsearch.yaml` - wait for it to report healthy before
   applying Kibana (Kibana's `elasticsearchRef` expects it to already exist).
3. `kubectl apply -f kibana.yaml`
4. Install `filebeat` via Helm (`../../helm/values/filebeat-values.yaml`) - references
   the `logs-es-elastic-user` / `logs-es-http-certs-public` Secrets ECK creates
   automatically in step 2, not anything from this repo.

## Not yet deployable

Needs Epic 4's cluster.
