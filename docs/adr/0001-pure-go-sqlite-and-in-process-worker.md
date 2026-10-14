# Pure-Go SQLite and In-Process Job Worker

We chose `modernc.org/sqlite` (pure Go, CGO_ENABLED=0) and an in-process worker queue rather than external infrastructure (PostgreSQL, Redis, Celery). This enables DocForge to compile into a single self-contained, statically linked binary while supporting ACID transactions, job lease polling, and local file persistence without requiring external service dependencies.
