import React, { FC, useState, useEffect } from "react";
import Container from "@mui/material/Container";
import Box from "@mui/material/Box";
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import CircularProgress from "@mui/material/CircularProgress";
import InputAdornment from "@mui/material/InputAdornment";
import Alert from "@mui/material/Alert";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogActions from "@mui/material/DialogActions";

import Page from "../components/system/Page";
import useAccount from "../hooks/useAccount";
import useRouter from "../hooks/useRouter";
import { TypesOrganization } from "../api/api";
import useSnackbar from "../hooks/useSnackbar";
import CopyButton from "../components/common/CopyButton";

const OrgSettings: FC = () => {
  // Get account context and router
  const account = useAccount();
  const router = useRouter();
  const snackbar = useSnackbar();

  // Form state
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [autoJoinDomain, setAutoJoinDomain] = useState("");
  const [loading, setLoading] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [slugManuallyEdited, setSlugManuallyEdited] = useState(false);
  const [errors, setErrors] = useState<{
    slug?: string;
    name?: string;
    autoJoinDomain?: string;
  }>({});

  const organization = account.organizationTools.organization;
  const isOrgOwner =
    !!account.user &&
    !!organization &&
    !!organization.memberships?.some(
      (membership) =>
        membership.user_id === account.user?.id && membership.role === "owner",
    );

  // Generate slug from name for new organizations
  const handleNameBlur = () => {
    // Only auto-generate slug if:
    // 1. Current slug is empty
    // 2. User hasn't manually edited the slug
    if (slug === "" && !slugManuallyEdited && name) {
      // Convert name to slug format: lowercase, replace spaces with hyphens
      const generatedSlug = name.toLowerCase().replace(/\s+/g, "-");
      setSlug(generatedSlug);
    }
  };

  // Mark slug as manually edited when user changes it
  const handleSlugChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSlugManuallyEdited(true);
    setSlug(e.target.value);
  };

  // Validate form before submission
  const validateForm = () => {
    const newErrors: { slug?: string; name?: string; autoJoinDomain?: string } =
      {};

    // Validate name (required)
    if (!name) {
      newErrors.name = "Name is required";
    }

    // Validate slug (required and no spaces)
    if (!slug) {
      newErrors.slug = "Slug is required";
    } else if (slug.includes(" ")) {
      newErrors.slug = "Slug cannot contain spaces";
    }

    // Validate auto-join domain if provided
    if (autoJoinDomain) {
      const domain = autoJoinDomain.trim().toLowerCase();
      if (domain.startsWith("@")) {
        newErrors.autoJoinDomain =
          "Domain should not start with @, use 'example.com' not '@example.com'";
      } else if (domain.includes("@")) {
        newErrors.autoJoinDomain =
          "Domain should not contain @, use 'example.com' not 'user@example.com'";
      } else if (
        !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/.test(
          domain,
        )
      ) {
        newErrors.autoJoinDomain = "Invalid domain format";
      }
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // Handle form submission
  const handleSubmit = async () => {
    // Validate form before submission
    if (!validateForm()) {
      return;
    }

    try {
      setLoading(true);

      if (!organization || !organization.id) {
        snackbar.error("Organization not found");
        return;
      }

      // Create the updated organization object
      const updatedOrg = {
        ...organization,
        name: slug, // 'name' field in API is our 'slug'
        display_name: name, // 'display_name' in API is our 'name'
        auto_join_domain: autoJoinDomain.trim().toLowerCase() || undefined,
      } as TypesOrganization;

      await account.organizationTools.updateOrganization(
        organization.id,
        updatedOrg,
      );
      snackbar.success("Organization updated successfully");

      if (slug != organization.name) {
        router.navigate("org_settings", { org_id: slug });
      }
    } catch (error) {
      console.error("Error updating organization:", error);
      snackbar.error("Failed to update organization");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (organization) {
      setSlug(organization.name || "");
      setName(organization.display_name || "");
      setAutoJoinDomain(organization.auto_join_domain || "");
    }
  }, [organization]);

  const handleDeleteOrganization = async () => {
    if (!organization?.id) {
      snackbar.error("Organization not found");
      return;
    }

    try {
      setDeleting(true);
      const deleted = await account.organizationTools.deleteOrganization(
        organization.id,
      );
      if (deleted) {
        setDeleteDialogOpen(false);
        const nextMemberOrg = account.organizationTools.organizations.find(
          (org) => org.id !== organization.id && org.member,
        );
        if (nextMemberOrg) {
          router.navigate("org_projects", {
            org_id: nextMemberOrg.name || nextMemberOrg.id || "",
          });
          return;
        }
        router.navigate("orgs");
      }
    } catch (error) {
      console.error("Error deleting organization:", error);
      snackbar.error("Failed to delete organization");
    } finally {
      setDeleting(false);
    }
  };

  if (!account.user) return null;
  // if(!account.isOrgMember) return null

  // Determine if the user can edit the organization settings
  const isReadOnly = !account.isOrgAdmin;

  return (
    <Page
      breadcrumbTitle={organization ? `Settings` : "Organization Settings"}
      breadcrumbParent={{
        title: "Organizations",
        routeName: "orgs",
        useOrgRouter: false,
      }}
      breadcrumbShowHome={true}
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3, p: 2 }}>
          <Typography variant="h5" component="h2" gutterBottom>
            Organization Settings
            {isReadOnly && (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ ml: 2 }}
              >
                (Read-only: Admin privileges required to make changes)
              </Typography>
            )}
          </Typography>

          {organization ? (
            <Box component="form" sx={{ mt: 3 }}>
              {/* Name field (formerly Display Name) */}
              <TextField
                label="Name"
                fullWidth
                value={name}
                onChange={(e) => setName(e.target.value)}
                onBlur={handleNameBlur}
                disabled={loading || isReadOnly}
                required
                error={!!errors.name}
                helperText={
                  errors.name || "Human-readable name for the organization"
                }
                sx={{ mb: 3 }}
                InputProps={{
                  readOnly: isReadOnly,
                }}
              />

              {/* Slug field (formerly Name) */}
              <TextField
                label="Slug"
                fullWidth
                value={slug}
                onChange={handleSlugChange}
                disabled={loading || isReadOnly}
                required
                error={!!errors.slug}
                helperText={
                  errors.slug ||
                  "Unique identifier for the organization (no spaces allowed)"
                }
                sx={{ mb: 3 }}
                InputProps={{
                  readOnly: isReadOnly,
                }}
              />

              {/* Organization ID field (read-only) */}
              <TextField
                label="Organization ID"
                fullWidth
                value={organization?.id || ""}
                disabled={true}
                helperText="Unique identifier for the organization"
                sx={{ mb: 3 }}
                InputProps={{
                  readOnly: true,
                  endAdornment: (
                    <InputAdornment position="end">
                      <CopyButton
                        content={organization?.id || ""}
                        title="Organization ID"
                      />
                    </InputAdornment>
                  ),
                }}
              />

              {/* Auto-Join Domain field */}
              <TextField
                label="Auto-Join Domain"
                fullWidth
                value={autoJoinDomain}
                onChange={(e) => setAutoJoinDomain(e.target.value)}
                disabled={loading || isReadOnly}
                error={!!errors.autoJoinDomain}
                helperText={
                  errors.autoJoinDomain ||
                  "Users logging in via OIDC with this email domain will automatically join this organization (e.g., 'acme.com')"
                }
                sx={{ mb: 2 }}
                placeholder="example.com"
                InputProps={{
                  readOnly: isReadOnly,
                }}
              />
              {autoJoinDomain && !errors.autoJoinDomain && (
                <Alert severity="info" sx={{ mb: 3 }}>
                  Users with <strong>@{autoJoinDomain.toLowerCase()}</strong>{" "}
                  email addresses will automatically be added as members when
                  they log in via OIDC (e.g., Google, Azure AD). This only works
                  for OIDC authentication with verified emails.
                </Alert>
              )}

              {!isReadOnly && (
                <Box
                  sx={{ mt: 3, display: "flex", justifyContent: "flex-end" }}
                >
                  <Button
                    onClick={handleSubmit}
                    variant="outlined"
                    color="secondary"
                    disabled={loading}
                    startIcon={loading ? <CircularProgress size={20} /> : null}
                  >
                    Update Organization
                  </Button>
                </Box>
              )}

              {isOrgOwner && (
                <Paper
                  elevation={0}
                  sx={{
                    mt: 4,
                    p: 3,
                    border: "1px solid",
                    borderColor: "error.main",
                    backgroundColor: "error.50",
                  }}
                >
                  <Typography variant="h6" color="error.main" gutterBottom>
                    Danger Zone
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    Deleting this organization permanently removes its projects,
                    teams, members, and repositories. This action cannot be
                    undone.
                  </Typography>
                  <Button
                    variant="contained"
                    color="error"
                    onClick={() => setDeleteDialogOpen(true)}
                    disabled={deleting}
                  >
                    Delete Organization
                  </Button>
                </Paper>
              )}
            </Box>
          ) : (
            <Box sx={{ display: "flex", justifyContent: "center", p: 3 }}>
              <CircularProgress />
            </Box>
          )}
        </Box>
      </Container>

      <Dialog
        open={deleteDialogOpen}
        onClose={() => !deleting && setDeleteDialogOpen(false)}
      >
        <DialogTitle>Delete organization?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This permanently deletes <strong>{organization?.display_name || organization?.name}</strong> and all of its data.
            This action cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setDeleteDialogOpen(false)}
            disabled={deleting}
          >
            Cancel
          </Button>
          <Button
            onClick={handleDeleteOrganization}
            color="error"
            variant="contained"
            disabled={deleting}
            startIcon={deleting ? <CircularProgress size={18} /> : null}
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Page>
  );
};

export default OrgSettings;
