# Keycloak Theme Development

This directory contains custom themes for Keycloak development.

## Setup

The docker-compose.dev.yaml has been configured for theme development with:
- Theme caching disabled (`KC_SPI_THEME_CACHE_THEMES=false`)
- Template caching disabled (`KC_SPI_THEME_CACHE_TEMPLATES=false`)
- Static resource caching disabled (`KC_SPI_THEME_STATIC_MAX_AGE=-1`)
- Themes directory mounted to `/opt/keycloak/themes`

## Current Themes

### custom-theme
- **Login theme**: Custom styling with gradient background and modern UI
- **Development indicator**: Shows "🚧 DEVELOPMENT THEME ACTIVE 🚧" in top-right corner

## Testing Your Theme

1. Start the development environment:
   ```bash
   docker-compose -f docker-compose.dev.yaml up keycloak postgres
   ```

2. Access Keycloak Admin Console:
   - URL: http://localhost:30080/auth/admin/
   - Username: admin
   - Password: `KEYCLOAK_ADMIN_PASSWORD` from your .env file (default: oh-hallo-insecure-password)

3. Configure the theme:
   - Go to Realm Settings > Themes
   - Set "Login Theme" to "custom-theme"
   - Click Save

4. Test the login page:
   - Go to http://localhost:30080/auth/realms/master/account/
   - You'll be redirected to the login page with your custom theme

## Development Workflow

1. Make changes to CSS/templates in the `themes/custom-theme/` directory
2. Refresh your browser - changes should appear immediately (no caching)
3. If changes don't appear, check browser developer tools for CSS loading errors

## Theme Structure

```
themes/custom-theme/
├── theme.properties              # Main theme configuration
├── login/
│   ├── theme.properties         # Login-specific configuration
│   └── resources/
│       └── css/
│           └── custom.css       # Custom styles
├── account/                     # Account theme (future)
├── admin/                       # Admin theme (future)
└── email/                       # Email theme (future)
```

## Customization Tips

- Edit `themes/custom-theme/login/resources/css/custom.css` for login page styling
- The development indicator helps confirm your theme is active
- Use browser developer tools to inspect elements and test CSS changes
- Reference the default Keycloak theme files for structure and available classes 