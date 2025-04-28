CREATE TABLE workflows (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create the tasks table
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
    execution_id TEXT,   
    PRIMARY KEY (id, workflow_id)
);

-- Create task_dependencies table
CREATE TABLE task_dependencies (
    task_id TEXT NOT NULL,
    workflow_id BIGINT NOT NULL,
    depends_on_task_id TEXT NOT NULL,
    PRIMARY KEY (task_id, workflow_id, depends_on_task_id),
    FOREIGN KEY (task_id, workflow_id) REFERENCES tasks (id, workflow_id) ON DELETE CASCADE,
    FOREIGN KEY (workflow_id) REFERENCES workflows (id) ON DELETE CASCADE
);
