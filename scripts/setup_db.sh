#!/bin/bash

set -e

echo "🚀 Setting up go-sql-store database..."

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose is not installed. Please install Docker and Docker Compose."
    exit 1
fi

# Check if .env exists, if not copy from example
if [ ! -f .env ]; then
    echo "📝 Creating .env file from .env.example..."
    cp .env.example .env
fi

# Start PostgreSQL
echo "🐘 Starting PostgreSQL container..."
docker-compose up -d

# Wait for PostgreSQL to be ready
echo "⏳ Waiting for PostgreSQL to be ready..."
sleep 5

# Check if PostgreSQL is ready
until docker-compose exec -T postgres pg_isready -U postgres > /dev/null 2>&1; do
    echo "⏳ Still waiting for PostgreSQL..."
    sleep 2
done

echo "✅ PostgreSQL is ready!"

# Run migrations
echo "🔄 Running migrations..."
go run scripts/run_migrations.go up

echo "✅ Database setup complete!"
echo ""
echo "Next steps:"
echo "  make run          # Start the API server"
echo "  make test         # Run integration tests"
echo ""
