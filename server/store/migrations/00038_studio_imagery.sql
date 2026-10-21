-- +goose Up
-- What Adx would ask an image model for, per concept: the scene the copy
-- sits on. Written by concept generation, editable by a person, read by
-- imagery generation. Empty until someone or Adx fills it.
ALTER TABLE creative_concepts ADD COLUMN imagery TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE creative_concepts DROP COLUMN imagery;
