#!/usr/bin/env bash
set -euo pipefail

NAME="${1:-}"
if [ -z "$NAME" ]; then
	echo "✗ usage: make seeder name=Post"
	exit 1
fi

if ! [[ "$NAME" =~ ^[A-Z][A-Za-z0-9]*$ ]]; then
	echo "✗ name must be PascalCase (e.g. Post, BlogPost)"
	exit 1
fi

SNAKE=$(echo "$NAME" | sed -E 's/([a-z0-9])([A-Z])/\1_\2/g' | tr '[:upper:]' '[:lower:]')
FILE="internal/database/seeders/${SNAKE}_seeder.go"

if [ -f "$FILE" ]; then
	echo "✗ $FILE already exists"
	exit 1
fi

cat > "$FILE" <<EOF
package seeders

import (
	"context"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// ${NAME}Seeder seeds data for the ${NAME} domain. Implementations MUST be
// idempotent — UPSERT or guard with existence checks.
type ${NAME}Seeder struct{}

func (${NAME}Seeder) Name() string { return "${SNAKE}" }

func (${NAME}Seeder) Run(ctx context.Context, db *sqlx.DB, l *zap.Logger) error {
	// TODO: implement seeding logic for ${NAME}.
	l.Info("${SNAKE} seeder: not implemented")
	return nil
}
EOF

echo "✓ created $FILE"
echo "→ register the new seeder in internal/database/seeders/database_seeder.go:"
echo "    seeders: []Seeder{ ..., ${NAME}Seeder{} },"
