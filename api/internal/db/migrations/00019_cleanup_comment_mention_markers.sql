-- +goose Up
-- Legacy comments used U+2063 INVISIBLE SEPARATOR as an in-band mention
-- boundary. Structured rows cannot be reconstructed safely from display names,
-- so remove the marker without guessing identities. Migration 00018 installs
-- the normalization invariant in a separate, short transaction first, so old
-- application instances and direct SQL writers cannot reintroduce markers
-- while or after this cleanup runs.
UPDATE comments
SET body = regexp_replace(body, chr(8291), '', 'g')
WHERE strpos(body, chr(8291)) > 0;

-- +goose Down
-- Intentionally irreversible: removed in-band markers cannot be reconstructed.
