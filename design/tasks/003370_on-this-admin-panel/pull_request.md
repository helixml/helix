# Show organization member roles in admin search results

## Summary
Preserve organization membership roles in the admin organizations API response and show each user's role alongside their email in the results table. Admins searching by member email can now immediately distinguish owners from members.

## Testing
- `go test ./api/pkg/server -run 'TestAdminListOrganizations|TestOrganizationSearchName' -count=1` — passed
- `yarn test src/components/dashboard/AdminOrgsTable.test.tsx --run` — passed
- `yarn tsc` — passed
