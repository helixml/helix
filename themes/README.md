# Keycloak Theme Development

This directory contains custom themes for Keycloak development.

## Production Deployment

In production, the Helix theme is baked into a custom Keycloak Docker image:
- **Image**: `registry.helixml.tech/helix/keycloak:latest`
- **Base**: `quay.io/keycloak/keycloak:23.0` (pinned for compatibility)
- **Theme**: Pre-built and cached for optimal performance

The custom image is automatically built and pushed by the CI/CD pipeline when changes are made to the theme files.

## Development Setup

The docker-compose.dev.yaml has been configured for theme development with:
- **Base Image**: `registry.helixml.tech/helix/keycloak:latest` (same as production)
- **Theme Override**: Local `./themes` directory mounted to `/opt/keycloak/themes`
- **Theme caching disabled** (`KC_SPI_THEME_CACHE_THEMES=false`)
- **Template caching disabled** (`KC_SPI_THEME_CACHE_TEMPLATES=false`)
- **Static resource caching disabled** (`KC_SPI_THEME_STATIC_MAX_AGE=-1`)

This setup ensures:
1. Development uses the same base image as production
2. Local theme changes are immediately visible (no caching)
3. The mounted volume overrides the baked-in theme for development

## Current Themes

### helix
- **Login theme**: Custom styling with gradient background, glass morphism effects, and modern UI
- **Background images**: Context-aware (charm.png for registration, particle.png for login)
- **Auto-configured**: The Keycloak reconciler automatically sets the realm's login theme to "helix"

## Testing Your Theme

1. Start the development environment:
   ```bash
   docker-compose -f docker-compose.dev.yaml up keycloak postgres
   ```

2. The theme is automatically configured by the Helix API server - no manual setup needed!

3. Test the login page:
   - Go to http://localhost:30080/auth/realms/helix/account/
   - You'll be redirected to the login page with the custom theme

## Development Workflow

1. Make changes to CSS/templates in the `themes/helix/` directory
2. Refresh your browser - changes should appear immediately (no caching)
3. If changes don't appear, check browser developer tools for CSS loading errors

## Theme Structure

```
themes/helix/
├── login/
│   ├── theme.properties         # Login-specific configuration
│   ├── template.ftl            # Main template with background images
│   ├── login.ftl               # Login form
│   ├── register.ftl            # Registration form
│   ├── login-reset-password.ftl # Password reset form
│   ├── info.ftl                # Info/success pages
│   ├── error.ftl               # Error pages
│   ├── logout.ftl              # Logout confirmation
│   └── resources/
│       ├── css/
│       │   └── custom.css      # Custom styles with glass morphism
│       └── img/
│           ├── charm.png       # Registration background
│           └── particle.png    # Login background
├── account/                    # Account theme (future)
├── admin/                      # Admin theme (future)
└── email/                      # Email theme (future)
```

## Docker Image Build

The custom Keycloak image is built using `Dockerfile.keycloak`.

The CI/CD pipeline in `.drone.yml` automatically:
1. Builds the image when theme files change
2. Pushes to `registry.helixml.tech/helix/keycloak`
3. Tags with version numbers for releases

## Customization Tips

- Edit `themes/helix/login/resources/css/custom.css` for styling changes
- Use browser developer tools to inspect elements and test CSS changes
- Background images are automatically selected based on page type
- Glass morphism effects are applied to forms and buttons for modern UI

 