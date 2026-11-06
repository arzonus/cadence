-- Warm storage: slim row of a closed run, keyed by run (not by shard).
CREATE TABLE closed_executions (
    domain_id     uuid,
    workflow_id   text,
    run_id        uuid,
    data          blob,
    data_encoding text,
    PRIMARY KEY ((domain_id, workflow_id), run_id)
) WITH compaction = {
    'class': 'org.apache.cassandra.db.compaction.LeveledCompactionStrategy'
};
