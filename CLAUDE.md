# Claude Development Notes

## Version Management
- **IMPORTANT**: Always commit when bumping the version
- When updating Docker containers with new package versions (e.g. step8.8 → step8.9), commit the changes to track version progression
- This helps maintain a clear history of version updates and associated fixes

## Project Structure
- HyprMoon packages are built in `/home/luke/pm/hyprmoon/`
- Docker containers use packages copied to `/home/luke/pm/helix/`
- Update both Dockerfile references and copy new packages when bumping versions

## Recent Work
- Fixed double-free vulnerabilities in WolfMoonlightServer (step8.9)
- Implemented RAII patterns with std::unique_ptr for memory safety
- Added thread-safe shutdown with mutex protection