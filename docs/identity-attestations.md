# Identity attestations

An identity attestation lets an external service learn which spec task, session
or sandbox is calling it without the caller handing over its Helix API key. A
pentest portal uses it to keep lanes apart: a finder lane cannot pass itself
off as the verifier lane of the same project, because the token states the
spec task the minting key is bound to, and nothing the lane sends can change
that.

## Flow

1. The verifier sends the lane a fresh nonce (16–128 characters of
   `[A-Za-z0-9._~+/=-]`) and names itself as the audience.
2. Inside its desktop the lane runs:

   ```bash
   helix identity attest --audience ptaas.example.com --nonce "$NONCE"
   ```

   This calls `POST /api/v1/identity/attestations` with the key Helix injected
   into the desktop (`USER_API_TOKEN`, `HELIX_API_URL`) and prints a token
   valid for five minutes.
3. The lane hands the token to the verifier.
4. The verifier redeems it once:

   ```bash
   helix identity verify --audience ptaas.example.com --nonce "$NONCE" --token-file token.txt
   ```

   or `POST /api/v1/identity/attestations/verify` with
   `{"token", "audience", "nonce"}`. The response is the claims:
   `spec_task_id`, `session_id` or `sandbox_id`, `project_id`,
   `organization_id`, `owner`, `aud`, `nonce`, `iat`, `exp`, `jti`.

## Who may call

- **Mint:** only the session or sandbox key of a desktop that belongs to a
  project. Personal, org, project and app keys, browser sessions, embed keys
  and bot instance keys get 403, as do keys whose session, spec task or
  sandbox is gone, no longer matches the key, or whose spec task is archived.
  A done task that is not archived still attests.
- **Verify:** a signed-in user or an org-scoped API key that may read the
  attested project. Session, sandbox, project and app keys get 403, so a lane
  cannot probe tokens.

Every failed verification — bad signature, expired, wrong audience or nonce,
a project the caller cannot read, another organization, or a token already
redeemed — returns the same `404 {"error":"attestation not valid"}`. A token
is redeemed only when every check passes, so a refused verifier cannot burn
it.

## Properties

- Claims come only from the minting key and the rows it is bound to.
- Tokens are HS256 JWTs signed with a key derived (HKDF-SHA256, dedicated
  label) from the server's JWT signing secret, with their own `kid`, `typ`,
  issuer and purpose claim. Session JWTs do not verify as attestations, and
  attestations do not authenticate API calls.
- Each `jti` is recorded on first verification and the record is kept until
  the token expires.
- Minting and verification each write a `project_audit_logs` row
  (`identity_attestation_minted`, `identity_attestation_verified`) with the
  jti and audience. Request bodies of both routes are kept out of the
  request audit log.
