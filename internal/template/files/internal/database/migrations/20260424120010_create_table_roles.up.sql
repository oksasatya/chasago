-- roles is a small lookup table. Rows are populated by RoleSeeder; the
-- DDL lives here because schema = migration, data = seeder.
CREATE TABLE roles (
    name        TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT ''
);
