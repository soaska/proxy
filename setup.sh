#!/bin/bash

# Proxi Statistics System Setup Script

echo "🚀 Setting up Proxi Statistics System"
echo ""

# Create data directory
mkdir -p data
echo "✅ Created data directory"

# Download GeoLite2 database
echo ""
echo "📥 GeoLite2-City database setup"
if [ ! -f data/GeoLite2-City.mmdb ]; then
    echo "⚠️  GeoLite2-City.mmdb not found in ./data/"
    echo ""
    echo "To enable GeoIP functionality, you need to:"
    echo "1. Register for a free MaxMind account at:"
    echo "   https://dev.maxmind.com/geoip/geolite2-free-geolocation-data"
    echo ""
    echo "2. Download GeoLite2-City.mmdb"
    echo ""
    echo "3. Place it in ./data/GeoLite2-City.mmdb"
    echo ""
    echo "The proxy will work without GeoIP, but geographic statistics will be disabled."
else
    echo "✅ GeoLite2-City.mmdb already exists"
fi

# Copy example config if config doesn't exist
echo ""
if [ ! -f config.yml ]; then
    echo "📝 Copying config.example.yml to config.yml"
    cp config.example.yml config.yml
    echo "✅ Created config.yml"
    echo "⚠️  Please edit config.yml with your settings"
else
    echo "✅ config.yml already exists"
fi

# Copy example env if .env doesn't exist
echo ""
if [ ! -f .env ]; then
    echo "📝 Copying .env.example to .env"
    cp .env.example .env
    echo "✅ Created .env"
    echo "⚠️  Please edit .env with your API key and Telegram bot token"
else
    echo "✅ .env already exists"
fi

# Check if speedtest is installed
echo ""
echo "🔍 Checking for Ookla Speedtest CLI..."
if command -v speedtest &> /dev/null; then
    echo "✅ Speedtest CLI is installed"
else
    echo "⚠️  Speedtest CLI not found"
    echo ""
    echo "To enable speedtest functionality, install Ookla Speedtest CLI:"
    echo "https://www.speedtest.net/apps/cli"
    echo ""
    echo "For Docker deployment, it will be installed automatically."
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Setup complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Next steps:"
echo ""
echo "1. 📥 Download GeoLite2-City.mmdb (if not already done)"
echo "   Place it in ./data/GeoLite2-City.mmdb"
echo ""
echo "2. ✏️  Edit config.yml with your configuration"
echo ""
echo "3. ✏️  Edit .env with your credentials:"
echo "   - API_KEY (for private API access)"
echo "   - TELEGRAM_BOT_TOKEN (get from @BotFather)"
echo "   - Your Telegram ID for admin access"
echo ""
echo "4. 🐳 Build and run with Docker:"
echo "   docker-compose up -d"
echo ""
echo "   Or run directly:"
echo "   go run ."
echo ""
echo "For more information, see IMPLEMENTATION_PLAN.md"
echo ""