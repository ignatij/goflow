CREATE TABLE workflows (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tasks (
    id VARCHAR(50) NOT NULL,
    workflow_id INT REFERENCES workflows(id),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    retries INT DEFAULT 0,
    attempts INT DEFAULT 0,
    error_msg TEXT,
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    PRIMARY KEY (id, workflow_id)
);
