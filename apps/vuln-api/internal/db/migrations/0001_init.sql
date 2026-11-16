-- VulnMan schema. Runs as vulnman_owner. vulnman_app (the running server)
-- gets exactly the grants at the bottom - in particular only SELECT and
-- INSERT on audit_log.

CREATE TABLE findings (
    id                bigserial PRIMARY KEY,
    ref               text NOT NULL UNIQUE,
    title             text NOT NULL,
    asset             text NOT NULL,
    severity          text NOT NULL CHECK (severity IN ('Critical','High','Medium','Low')),
    cvss              numeric(3,1) CHECK (cvss >= 0 AND cvss <= 10),
    cve               text NOT NULL DEFAULT '',
    status            text NOT NULL CHECK (status IN ('New','Triaged','In Progress','Remediated','Accepted Risk')),
    source            text NOT NULL CHECK (source IN ('internal','external','trivy','csv')),
    description_ct    bytea,
    reporter_email_ct bytea,
    wrapped_dek       text NOT NULL,
    assignee_sub      text NOT NULL DEFAULT '',
    assignee_name     text NOT NULL DEFAULT '',
    due_date          timestamptz,
    discovered_at     timestamptz NOT NULL,
    triaged_at        timestamptz,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL,
    fingerprint       text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX findings_open_fingerprint ON findings (fingerprint)
    WHERE fingerprint <> '' AND status NOT IN ('Remediated','Accepted Risk');
CREATE INDEX findings_status_severity ON findings (status, severity);
CREATE INDEX findings_assignee ON findings (assignee_sub);

CREATE TABLE finding_links (
    a    bigint NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    b    bigint NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('duplicate','related','same_root_cause')),
    PRIMARY KEY (a, b),
    CHECK (a < b)
);

CREATE TABLE attachments (
    id           bigserial PRIMARY KEY,
    finding_id   bigint NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    name         text NOT NULL,
    content_type text NOT NULL,
    size         integer NOT NULL,
    data_ct      bytea NOT NULL,
    created_by   text NOT NULL,
    created_at   timestamptz NOT NULL
);

CREATE TABLE sla_targets (
    severity text PRIMARY KEY CHECK (severity IN ('Critical','High','Medium','Low')),
    days     integer NOT NULL CHECK (days > 0)
);
INSERT INTO sla_targets VALUES ('Critical', 7), ('High', 30), ('Medium', 90), ('Low', 180);

CREATE TABLE sessions (
    id_hash      bytea PRIMARY KEY,
    sub          text NOT NULL,
    name         text NOT NULL,
    email        text NOT NULL,
    role         text NOT NULL,
    created_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL
);
CREATE INDEX sessions_sub ON sessions (sub);

CREATE TABLE audit_log (
    id         bigserial PRIMARY KEY,
    at         timestamptz NOT NULL,
    actor_sub  text NOT NULL,
    actor_role text NOT NULL,
    action     text NOT NULL,
    finding_id bigint,
    details    jsonb NOT NULL,
    prev_hash  bytea,
    hash       bytea NOT NULL
);
CREATE INDEX audit_log_finding ON audit_log (finding_id);

-- Defence in depth on top of the grants below: even the owner can't
-- rewrite history without first dropping this trigger.
CREATE FUNCTION audit_log_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END $$;
CREATE TRIGGER audit_log_immutable BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_immutable();

GRANT SELECT, INSERT, UPDATE, DELETE ON findings, finding_links, attachments, sla_targets, sessions TO vulnman_app;
GRANT SELECT, INSERT ON audit_log TO vulnman_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO vulnman_app;
