-- +goose Up
CREATE TABLE projects (
    id         INTEGER PRIMARY KEY,
    name       TEXT     NOT NULL UNIQUE COLLATE NOCASE,
    created_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE projects;
