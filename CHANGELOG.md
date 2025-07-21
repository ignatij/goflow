## [0.2.3] - 2025-07-21

### Bug Fixes
- use go max procs as number for the buffered channel size (#16)


## [0.2.2] - 2025-07-21

### Bug Fixes
- fix release script


## [0.2.1] - 2025-07-21

### Bug Fixes
- fix release script
- test release workflow with version prefix handling


## [0.2.0] - 2025-07-21

### Features
- add context on execution; add method for updating task attempts; add logs (#10)
- introduce task timeouts (#8)
- add task retries
- add initial implementation of parallel workers
- add initial version of worker pool
- add worker pool initial implementation
- add task dependencies table
- mv workflow service to pkg; registering tasks/flows
- add the option of updating workflow status
- add dockerfile; add cli separation; add e2e tests

### Bug Fixes
- error propagation of dependent tasks (#15)
- error propagation of dependent tasks (#14)
- include task timeout in the retries of the task; docs: add examples (#11)
- make context aware from root context; execute flows for failed t… (#9)
- remove starting migration on server start
- add removed pq adapter
- all tests enabled
- replace strings with task status
- use enum for task status where missed
- introduce workflow ctx as shared state across the whole workflow
- try to use paralel for server_test
- lint
- remove deps
- remove deps as not needed in db
- argument parsing
- fallback to env vars in main when --db flat is not set
- add binary go .gitignore; tidy deps
- adapt creation of workflow to be a json form
- switch to json response in http handlers
- add int64
- use host from the running container
- remove coverage.out from repo
- use test command directly
- run only test without init local db
- not needed prints; rely on env vars instead of error if .env missing

### Documentation
- add readme and contributing files (#12)

### Maintenance
- add sem ver releasing (#13)
- execute migration on CI (#7)
- execute db migrations on server start (#6)
- add topological sorting and registration of tasks/flows
- include begin/commit/rollback to store
- add initial commit

### Other Changes
- introduce task status


# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2024-01-01

### Features
- Initial release of GoFlow
- Task dependency management
- Parallel task execution
- Retry mechanism with configurable policies
- Timeout support for tasks
- PostgreSQL storage backend
- HTTP API for workflow management
- CLI interface for workflow operations
- Docker support
- Comprehensive test coverage

### Documentation
- Complete README with library and service usage examples
- Contributing guidelines
- API documentation
- Example workflows demonstrating various features 
