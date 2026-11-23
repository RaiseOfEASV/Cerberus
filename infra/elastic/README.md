# Elastic Cloud: pentest dashboard

`pentest-queries.json` holds the ES|QL queries behind the **Pentest – App
Security** dashboard. `pentest-dashboard.ndjson` is the Kibana saved-objects
export generated from them (8 saved searches and the dashboard). It also
references three saved searches created by hand in Kibana: Successful
Logins, Failed Logins and Successful Logins after Failure.

Object ids are fixed (UUIDv5), so importing again overwrites only these
objects:

```sh
curl -X POST "$KIBANA_URL/api/saved_objects/_import?overwrite=true" \
  -H "Authorization: ApiKey $KIBANA_API_KEY" -H "kbn-xsrf: true" \
  -F file=@infra/elastic/pentest-dashboard.ndjson
```

The queries read the `json.*` fields that the Kubernetes integration's
`decode_json_fields` processor adds for the app namespaces (Fleet policy),
and rely on the `logs-kubernetes.container_logs@custom` component template
that types those fields. `json.ip` is `ip` in the first backing index and
`keyword` in later ones, so the queries use `TO_STRING(json.ip)`.
