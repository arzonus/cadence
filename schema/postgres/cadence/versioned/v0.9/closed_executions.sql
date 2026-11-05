CREATE TABLE closed_executions (
  domain_id     BYTEA        NOT NULL,
  workflow_id   TEXT         NOT NULL,
  run_id        BYTEA        NOT NULL,
  --
  data          BYTEA        NOT NULL,
  data_encoding VARCHAR(16)  NOT NULL,
  PRIMARY KEY (domain_id, workflow_id, run_id)
);
