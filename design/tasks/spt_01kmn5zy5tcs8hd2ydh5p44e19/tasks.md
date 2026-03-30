# Implementation Tasks

- [~] In `api/pkg/server/provider_handlers.go`, inside `updateProviderEndpoint`, after the admin check for global type (~line 481), add logic to apply `updatedEndpoint.EndpointType` to `existingEndpoint` and update `Owner`/`OwnerType` accordingly (`global` → owner=`"system"`, ownerType=`system`; `user` → owner=`user.ID`, ownerType=`user`)
- [ ] Verify the frontend `AddProviderDialog.tsx` sends the correct `endpoint_type` value in the update payload (should already work; confirm and fix if not)
- [ ] Test end-to-end: create a user endpoint, edit it as admin switching to global, confirm it appears in the provider list for another user and is included in the system refresh query
