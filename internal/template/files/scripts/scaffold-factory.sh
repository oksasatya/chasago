#!/usr/bin/env bash
set -euo pipefail

NAME="${1:-}"
if [ -z "$NAME" ]; then
	echo "✗ usage: make factory name=Post"
	exit 1
fi

if ! [[ "$NAME" =~ ^[A-Z][A-Za-z0-9]*$ ]]; then
	echo "✗ name must be PascalCase (e.g. Post, BlogPost)"
	exit 1
fi

SNAKE=$(echo "$NAME" | sed -E 's/([a-z0-9])([A-Z])/\1_\2/g' | tr '[:upper:]' '[:lower:]')
FILE="internal/database/factories/${SNAKE}_factory.go"

mkdir -p "$(dirname "$FILE")"

if [ -f "$FILE" ]; then
	echo "✗ $FILE already exists"
	exit 1
fi

cat > "$FILE" <<EOF
package factories

import (
	"github.com/brianvoe/gofakeit/v7"
)

// ${NAME}Attrs is the shape produced by ${NAME}Factory. Replace fields with
// the columns you actually need to seed for the ${NAME} domain.
type ${NAME}Attrs struct {
	// e.g. Title string
}

// ${NAME}Factory builds ${NAME}Attrs values populated with fake data — mirror
// of Laravel's factory pattern.
type ${NAME}Factory struct {
	count     int
	overrides func(*${NAME}Attrs)
}

func New${NAME}() *${NAME}Factory { return &${NAME}Factory{count: 1} }

func (f *${NAME}Factory) Count(n int) *${NAME}Factory {
	f.count = n
	return f
}

func (f *${NAME}Factory) State(fn func(*${NAME}Attrs)) *${NAME}Factory {
	f.overrides = fn
	return f
}

func (f *${NAME}Factory) Make() []*${NAME}Attrs {
	out := make([]*${NAME}Attrs, 0, f.count)
	for i := 0; i < f.count; i++ {
		a := &${NAME}Attrs{
			// TODO: populate via gofakeit, e.g.:
			// Title: gofakeit.Sentence(5),
		}
		_ = gofakeit.Bool // remove once a gofakeit call is used above
		if f.overrides != nil {
			f.overrides(a)
		}
		out = append(out, a)
	}
	return out
}
EOF

echo "✓ created $FILE"
