import React, {
  useState,
  useCallback,
  useEffect,
  useMemo,
} from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Fade from "@mui/material/Fade";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import ButtonBase from "@mui/material/ButtonBase";
import Stack from "@mui/material/Stack";

import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import RadioButtonUncheckedIcon from "@mui/icons-material/RadioButtonUnchecked";
import BusinessIcon from "@mui/icons-material/Business";
import PersonIcon from "@mui/icons-material/Person";
import CreateNewFolderIcon from "@mui/icons-material/CreateNewFolder";
import CloseIcon from "@mui/icons-material/Close";
import IconButton from "@mui/material/IconButton";
import { Server } from "lucide-react";

import useAccount from "../hooks/useAccount";
import useApi from "../hooks/useApi";
import useLightTheme from "../hooks/useLightTheme";
import useSnackbar from "../hooks/useSnackbar";
import useRouter from "../hooks/useRouter";
import { SELECTED_ORG_STORAGE_KEY } from "../utils/localStorage";
import { useCreateOrg } from "../services/orgService";
import ClaudeSubscriptionConnect, {
  useClaudeSubscriptions,
} from "../components/account/ClaudeSubscriptionConnect";
import AnthropicLogo from "../components/providers/logos/anthropic";
import AgentHarness from "../components/agent/AgentHarness";
import CodexSubscriptionConnect from "../components/account/CodexSubscriptionConnect";
import { useGetConfig } from "../services/userService";
import { useGetWallet } from "../services/useBilling";
import CreditCardIcon from "@mui/icons-material/CreditCard";
import { useCodexSubscriptions } from "../services/codexSubscriptionsService";
import {
  CLAUDE_SUBSCRIPTION_MODELS,
  CODEX_SUBSCRIPTION_MODELS,
} from "../components/agent/CodingAgentForm";
import {
  findHarnessStatus,
  useOrgCodeAgentHarnesses,
  useUpdateOrgCodeAgentHarnesses,
} from "../services/codeAgentHarnessesService";
import { useListProviders } from "../services/providersService";
import {
  providersForCodeAgentHarness,
} from "../utils/codeAgentProviders";
import { providerRef } from "../components/create/AdvancedModelPicker";
import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
} from "../api/api";
import type { TypesCodeAgentExecutionConfig } from "../api/api";

const ACCENT = "#00e891";
const ACCENT_DIM = "rgba(0, 232, 145, 0.08)";
const CARD_BORDER_ACTIVE = "rgba(0, 232, 145, 0.25)";

function getOnboardingPalette(isLight: boolean) {
  return {
    BG: isLight ? "#f5f5f7" : "#0d0d1a",
    CARD_BG: isLight ? "#ffffff" : "#0f0f1e",
    CARD_BG_ACTIVE: isLight ? "#fafafa" : "#101024",
    CARD_BORDER: isLight ? "rgba(0,0,0,0.08)" : "rgba(255,255,255,0.04)",

    MENU_BG: isLight ? "#ffffff" : "#1a1a2e",
    MENU_TEXT: isLight ? "#1a1a2e" : "#fff",

    STEP_INACTIVE: isLight ? "rgba(0,0,0,0.2)" : "rgba(255,255,255,0.15)",

    TEXT_PRIMARY: isLight ? "#1a1a2e" : "#fff",
    TEXT_SECONDARY: isLight ? "rgba(0,0,0,0.65)" : "rgba(255,255,255,0.6)",
    TEXT_MUTED: isLight ? "rgba(0,0,0,0.55)" : "rgba(255,255,255,0.5)",
    TEXT_FADED: isLight ? "rgba(0,0,0,0.4)" : "rgba(255,255,255,0.4)",
    TEXT_DIM: isLight ? "rgba(0,0,0,0.35)" : "rgba(255,255,255,0.3)",

    BORDER_SUBTLE: isLight ? "rgba(0,0,0,0.06)" : "rgba(255,255,255,0.06)",
    BORDER_HOVER: isLight ? "rgba(0,0,0,0.2)" : "rgba(255,255,255,0.15)",
    INPUT_BORDER: isLight ? "rgba(0,0,0,0.2)" : "rgba(255,255,255,0.1)",
    INPUT_BORDER_HOVER: isLight ? "rgba(0,0,0,0.4)" : "rgba(255,255,255,0.2)",

    OVERLAY_FAINT: isLight ? "rgba(0,0,0,0.02)" : "rgba(255,255,255,0.02)",
    OVERLAY_DIM: isLight ? "rgba(0,0,0,0.03)" : "rgba(255,255,255,0.03)",

    RADIO_UNCHECKED: isLight ? "rgba(0,0,0,0.4)" : "rgba(255,255,255,0.3)",

    inputSx: {
      color: isLight ? "#1a1a2e" : "#fff",
      fontSize: "0.82rem",
      "& fieldset": { borderColor: isLight ? "rgba(0,0,0,0.2)" : "rgba(255,255,255,0.1)" },
      "&:hover fieldset": { borderColor: isLight ? "rgba(0,0,0,0.4)" : "rgba(255,255,255,0.2)" },
      "&.Mui-focused fieldset": { borderColor: ACCENT },
    },
    labelSx: { color: isLight ? "rgba(0,0,0,0.55)" : "rgba(255,255,255,0.4)", fontSize: "0.82rem" },
    helperSx: { color: isLight ? "rgba(0,0,0,0.4)" : "rgba(255,255,255,0.25)", fontSize: "0.72rem" },
    selectSx: {
      color: isLight ? "#1a1a2e" : "#fff",
      fontSize: "0.82rem",
      "& fieldset": { borderColor: isLight ? "rgba(0,0,0,0.2)" : "rgba(255,255,255,0.1)" },
      "&:hover fieldset": { borderColor: isLight ? "rgba(0,0,0,0.4)" : "rgba(255,255,255,0.2)" },
      "&.Mui-focused fieldset": { borderColor: ACCENT },
      "& .MuiSvgIcon-root": { color: isLight ? "rgba(0,0,0,0.4)" : "rgba(255,255,255,0.4)" },
    },
  };
}

const formatUnixTimestamp = (unixTs?: number) => {
  if (!unixTs || unixTs <= 0) return "—";
  return new Date(unixTs * 1000).toLocaleString();
};

const btnSx = {
  bgcolor: ACCENT,
  color: "#000",
  fontWeight: 600,
  px: 2.5,
  py: 0.8,
  borderRadius: 1.5,
  textTransform: "none" as const,
  fontSize: "0.8rem",
  "&:hover": { bgcolor: "#00cc7a" },
  "&.Mui-disabled": {
    bgcolor: "rgba(0,232,145,0.3)",
    color: "rgba(0,0,0,0.5)",
  },
};

const SUBSCRIPTION_BENEFITS = [
  "Full Linux desktop sandboxes where your agents can work safely",
  "Your entire subscription fee becomes credits for running AI models",
  "Bring your own Claude and Codex subscriptions",
  "Collaborate with your team, set budgets, and track usage",
];

type CodingAccessOption = "helix" | "claude" | "codex";

// Step type identifiers - used for conditional rendering and step content matching
type StepType =
  | "signin"
  | "organization"
  | "subscription"
  | "provider";

interface StepConfig {
  type: StepType;
  icon: React.ReactNode;
  title: string;
  subtitle: string;
}

const ALL_STEPS: StepConfig[] = [
  {
    type: "signin",
    icon: <PersonIcon />,
    title: "Sign in with your account",
    subtitle: "To get started, please sign in with your account credentials.",
  },
  {
    type: "organization",
    icon: <BusinessIcon />,
    title: "Set up your organization",
    subtitle:
      "Organizations help you collaborate with your team and manage projects together.",
  },
  {
    type: "subscription",
    icon: <CreditCardIcon />,
    title: "Activate subscription",
    subtitle: "Add payment method to activate your organization subscription.",
  },
  {
    type: "provider",
    icon: <Server size={20} />,
    title: "Choose how to run coding agents",
    subtitle: "Use Helix credits or connect an existing coding subscription.",
  },
];

export default function Onboarding() {
  const account = useAccount();
  const api = useApi();
  const snackbar = useSnackbar();
  const router = useRouter();
  const lightTheme = useLightTheme();
  const palette = getOnboardingPalette(lightTheme.isLight);

  // Step tracking
  const [activeStep, setActiveStep] = useState(1);
  const [completedSteps, setCompletedSteps] = useState<Set<number>>(
    new Set([0]),
  );

  // Server config and wallet (for subscription check)
  const { data: serverConfig, isLoading: isLoadingServerConfig } =
    useGetConfig();
  const [isSubscribing, setIsSubscribing] = useState(false);

  // Step 1: Organization
  const [orgMode, setOrgMode] = useState<"select" | "create">("select");
  const [selectedOrgId, setSelectedOrgId] = useState<string>("");
  const [orgDisplayName, setOrgDisplayName] = useState("");
  const [createdOrg, setCreatedOrg] = useState<{
    id: string;
    name: string;
    display_name?: string;
    viewer_is_owner: boolean;
  } | null>(null);
  const createOrgMutation = useCreateOrg();

  // Step 2: Subscription (conditional)
  const {
    data: wallet,
    refetch: refetchWallet,
    isFetching: isFetchingWallet,
  } = useGetWallet(
    createdOrg?.id,
    !!createdOrg?.id && !isLoadingServerConfig && serverConfig?.billing_enabled,
  );
  const isTrialing = wallet?.subscription_status === "trialing";
  const isSubscriptionActive =
    wallet?.subscription_status === "active" || isTrialing;

  // Final step: choose Helix credits or an external coding subscription.
  const [codingAccessOption, setCodingAccessOption] =
    useState<CodingAccessOption>("helix");
  const [claudeModel, setClaudeModel] = useState("");
  const [codexModel, setCodexModel] = useState("");
  const [helixProvider, setHelixProvider] = useState("");
  const [helixModel, setHelixModel] = useState("");
  const [helixReasoningEffort, setHelixReasoningEffort] = useState("none");
  const [finishingOnboarding, setFinishingOnboarding] = useState(false);
  const updateCodeAgentHarnesses = useUpdateOrgCodeAgentHarnesses(createdOrg?.id);
  const { data: providers = [], isLoading: providersLoading } = useListProviders({
    loadModels: true,
    orgId: createdOrg?.id,
    enabled: !!createdOrg?.id,
  });
  const { data: harnesses = [], isLoading: harnessesLoading } =
    useOrgCodeAgentHarnesses(createdOrg?.id, { enabled: !!createdOrg?.id });
  const zedHarness = findHarnessStatus(
    harnesses,
    TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
  );
  const helixProviders = providersForCodeAgentHarness(
    providers,
    zedHarness,
    TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
  );
  const selectedHelixProvider = helixProviders.find((provider) => providerRef(provider) === helixProvider);
  const helixModels = (selectedHelixProvider?.available_models || []).filter((model) =>
    model.id && model.enabled && (!model.type || model.type === "chat" || model.type === "text"));
  const helixDefaultAvailable = !!selectedHelixProvider
    && helixModels.some((model) => model.id === helixModel);
  const hasConfiguredHelixDefault = !!serverConfig?.onboarding_helix_model_provider
    && !!serverConfig?.onboarding_helix_model;

  useEffect(() => {
    if (!hasConfiguredHelixDefault) return;
    setHelixProvider(serverConfig.onboarding_helix_model_provider || "");
    setHelixModel(serverConfig.onboarding_helix_model || "");
    setHelixReasoningEffort(serverConfig.onboarding_helix_model_effort || "none");
  }, [
    hasConfiguredHelixDefault,
    serverConfig?.onboarding_helix_model_effort,
    serverConfig?.onboarding_helix_model_provider,
    serverConfig?.onboarding_helix_model,
  ]);

  const existingOrgs = account.organizationTools.organizations;
  const hasExistingOrgs = existingOrgs.length > 0;

  // External coding subscription state
  const { data: claudeSubscriptions } = useClaudeSubscriptions();
  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0;
  const { data: codexSubscriptions } = useCodexSubscriptions();
  const hasCodexSubscription = (codexSubscriptions?.length ?? 0) > 0;

  // Billing is optional on self-hosted installations; coding access is always
  // shown because it is the final onboarding choice.
  const visibleSteps = useMemo(() => {
    let steps = ALL_STEPS;
    if (!serverConfig?.billing_enabled) {
      steps = steps.filter((step) => step.type !== "subscription");
    }
    return steps;
  }, [serverConfig?.billing_enabled]);

  // Helper to get step index by type (in the visible steps array)
  const getStepIndexByType = useCallback(
    (type: StepType): number => {
      return visibleSteps.findIndex((step) => step.type === type);
    },
    [visibleSteps],
  );

  // Helper to get step type by index (in the visible steps array)
  const getStepTypeByIndex = useCallback(
    (index: number): StepType | undefined => {
      return visibleSteps[index]?.type;
    },
    [visibleSteps],
  );

  // Refetch wallet when organization is selected/created
  useEffect(() => {
    if (createdOrg?.id && serverConfig?.billing_enabled) {
      refetchWallet();
    }
  }, [createdOrg?.id, serverConfig?.billing_enabled, refetchWallet]);

  // Check for successful payment return from Stripe
  useEffect(() => {
    const url = new URL(window.location.href);
    const success = url.searchParams.get("success");
    if (success === "true") {
      refetchWallet();
      const subscriptionStepIndex = getStepIndexByType("subscription");
      if (subscriptionStepIndex >= 0) {
        setActiveStep(subscriptionStepIndex);
        setCompletedSteps((prev) => {
          const next = new Set(prev);
          next.delete(subscriptionStepIndex);
          return next;
        });
      }
      url.searchParams.delete("success");
      const nextUrl = `${url.pathname}${url.searchParams.toString() ? `?${url.searchParams.toString()}` : ""}`;
      window.history.replaceState({}, "", nextUrl);
    }
  }, [refetchWallet, getStepIndexByType]);

  useEffect(() => {
    const orgIdFromUrl = new URLSearchParams(window.location.search).get(
      "org_id",
    );
    if (!orgIdFromUrl || createdOrg || !existingOrgs.length) return;

    const org = existingOrgs.find((candidate) => candidate.id === orgIdFromUrl);
    if (!org?.id || !org?.name) return;

    setSelectedOrgId(org.id);
    setCreatedOrg({
      id: org.id,
      name: org.name,
      display_name: org.display_name,
      viewer_is_owner: org.owner === account.user?.id
        || !!org.memberships?.some((membership) =>
          membership.user_id === account.user?.id && membership.role === "owner"),
    });
    const orgStepIndex = getStepIndexByType("organization");
    setCompletedSteps((prev) => new Set([...prev, orgStepIndex]));
    setActiveStep(orgStepIndex + 1);
  }, [account.user?.id, createdOrg, existingOrgs, getStepIndexByType]);

  useEffect(() => {
    if (hasExistingOrgs) {
      setOrgMode("select");
      if (!selectedOrgId && existingOrgs[0]?.id) {
        setSelectedOrgId(existingOrgs[0].id);
      }
    } else {
      setOrgMode("create");
    }
  }, [hasExistingOrgs, existingOrgs]);

  const markComplete = useCallback((step: number) => {
    if (step < 0) return;
    setCompletedSteps((prev) => new Set([...prev, step]));
    setActiveStep(step + 1);
  }, []);

  const markStepCompleteByType = useCallback(
    (stepType: StepType) => {
      const stepIndex = getStepIndexByType(stepType);
      markComplete(stepIndex);
    },
    [getStepIndexByType, markComplete],
  );

  const handleComplete = useCallback(
    async () => {
      if (!createdOrg?.name) {
        snackbar.error("No organization selected");
        return;
      }

      const codeAgentConfig: TypesCodeAgentExecutionConfig | undefined =
        codingAccessOption === "claude"
          ? {
              runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
              credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
              model: claudeModel,
            }
          : codingAccessOption === "codex"
            ? {
                runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
                credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
                model: codexModel,
              }
            : helixProvider && helixModel
              ? {
                  runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
                  credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
                  provider_ref: helixProvider,
                  model: helixModel,
                  reasoning_effort: helixReasoningEffort,
                }
              : undefined;
      if (!codeAgentConfig) {
        snackbar.error("Select a complete default runtime configuration");
        return;
      }
      if (codingAccessOption === "helix" && !helixDefaultAvailable) {
        snackbar.error("The selected Helix model is not available for Zed Agent in this organization");
        return;
      }
      if (!createdOrg.viewer_is_owner) {
        snackbar.error("Ask an organization owner to set the Default Runtime");
        return;
      }

      const selectedHarness = findHarnessStatus(harnesses, codeAgentConfig.runtime);
      const subscriptionPolicyReady = selectedHarness?.enabled
        && selectedHarness.subscription_enabled === true;

      setFinishingOnboarding(true);

      try {
        if (codeAgentConfig.credential_type === TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
          && !subscriptionPolicyReady) {
          await updateCodeAgentHarnesses.mutateAsync([{
            runtime: codeAgentConfig.runtime!,
            enabled: true,
            subscription_enabled: true,
          }]);
        }
        await api.getApiClient().v1OrgsSettingsUpdate(
          "agent.default",
          createdOrg.name,
          { value: JSON.stringify({
            code_agent_runtime: codeAgentConfig.runtime,
            code_agent_credential_type: codeAgentConfig.credential_type,
            provider: codeAgentConfig.provider_ref || "",
            model: codeAgentConfig.model,
            reasoning_effort: codeAgentConfig.reasoning_effort || "none",
          }) },
        );
        try {
          await api.getApiClient().v1UsersMeOnboardingCreate();
        } catch (err) {
          console.error("Failed to mark onboarding complete:", err);
        }
        account.dismissOnboarding();
        localStorage.setItem(SELECTED_ORG_STORAGE_KEY, createdOrg.name);
        router.navigateReplace("org_projects", {
          org_id: createdOrg.name,
          create_project_config: JSON.stringify(codeAgentConfig),
        });
      } catch (err) {
        console.error("Failed to configure coding access:", err);
        snackbar.error("Failed to configure coding access");
      } finally {
        setFinishingOnboarding(false);
      }
    },
    [
      account,
      api,
      claudeModel,
      codexModel,
      codingAccessOption,
      createdOrg,
      harnesses,
      helixModel,
      helixProvider,
      helixReasoningEffort,
      helixDefaultAvailable,
      router,
      snackbar,
      updateCodeAgentHarnesses,
    ],
  );

  const handleSelectExistingOrg = useCallback(() => {
    if (!selectedOrgId) {
      snackbar.error("Please select an organization");
      return;
    }
    const org = existingOrgs.find((o) => o.id === selectedOrgId);
    if (!org?.id) {
      snackbar.error("Could not find the selected organization");
      return;
    }
    setCreatedOrg({
      id: org.id!,
      name: org.name!,
      display_name: org.display_name,
      viewer_is_owner: org.owner === account.user?.id
        || !!org.memberships?.some((membership) =>
          membership.user_id === account.user?.id && membership.role === "owner"),
    });
    markStepCompleteByType("organization");
  }, [account.user?.id, selectedOrgId, existingOrgs, markStepCompleteByType, snackbar]);

  const handleCreateOrg = useCallback(async () => {
    if (!orgDisplayName.trim()) {
      snackbar.error("Please enter an organization name");
      return;
    }
    try {
      const newOrg = await createOrgMutation.mutateAsync({
        display_name: orgDisplayName.trim(),
      });
      if (newOrg?.id) {
        setCreatedOrg({
          id: newOrg.id!,
          name: newOrg.name!,
          display_name: newOrg.display_name,
          viewer_is_owner: true,
        });
        await account.organizationTools.loadOrganizations();
        markStepCompleteByType("organization");
      }
    } catch (err) {
      console.error("Failed to create org:", err);
      snackbar.error("Failed to create organization");
    }
  }, [
    orgDisplayName,
    createOrgMutation,
    account.organizationTools,
    markStepCompleteByType,
    snackbar,
  ]);

  // Step 2: Activate subscription
  const handleSubscribe = useCallback(async () => {
    if (!createdOrg?.id) {
      snackbar.error("Organization not found");
      return;
    }

    try {
      setIsSubscribing(true);

      const resp = await api.getApiClient().v1SubscriptionNewCreate({
        org_id: createdOrg.id,
        return_url: `/onboarding?org_id=${createdOrg.id}`,
      });
      if (!resp.data) return;

      document.location = resp.data;
    } catch (error) {
      console.error("Subscription error:", error);
      snackbar.error("Failed to start subscription process");
    } finally {
      setIsSubscribing(false);
    }
  }, [api, createdOrg, snackbar]);

  const handleDismiss = useCallback(async () => {
    account.dismissOnboarding();
    try {
      await api.getApiClient().v1UsersMeOnboardingCreate();
    } catch (err) {
      console.error("Failed to mark onboarding complete on dismiss:", err);
    }
    const org = account.organizationTools.organization;
    if (org) {
      router.navigateReplace("org_projects", { org_id: org.name });
    }
  }, [api, router]);

  const userName =
    account.user?.name?.trim() ||
    account.user?.email?.split("@")[0] ||
    "there";

  const isStepCompleted = (step: number) => completedSteps.has(step);
  const isStepActive = (step: number) => activeStep === step;
  const isStepLocked = (step: number) => step > activeStep;

  const renderStepIcon = (step: number) => {
    if (isStepCompleted(step)) {
      return (
        <CheckCircleIcon
          sx={{
            fontSize: 24,
            color: ACCENT,
            filter: `drop-shadow(0 0 6px ${ACCENT}60)`,
          }}
        />
      );
    }
    return (
      <RadioButtonUncheckedIcon
        sx={{
          fontSize: 24,
          color: isStepActive(step) ? ACCENT : palette.STEP_INACTIVE,
        }}
      />
    );
  };

  const renderStepContent = (stepIndex: number) => {
    const stepType = getStepTypeByIndex(stepIndex);
    if (!stepType) return null;

    switch (stepType) {
      case "organization":
        return (
          <Fade in={isStepActive(stepIndex)} timeout={400}>
            <Box sx={{ mt: 2.5 }}>
              {hasExistingOrgs && (
                <Box sx={{ display: "flex", gap: 1.5, mb: 2.5 }}>
                  <Box
                    onClick={() => setOrgMode("select")}
                    sx={{
                      flex: 1,
                      p: 1.5,
                      borderRadius: 1.5,
                      border: `1px solid ${orgMode === "select" ? CARD_BORDER_ACTIVE : palette.CARD_BORDER}`,
                      bgcolor:
                        orgMode === "select" ? ACCENT_DIM : "transparent",
                      cursor: "pointer",
                      transition: "all 0.2s",
                      "&:hover": { borderColor: palette.BORDER_HOVER },
                    }}
                  >
                    <Box
                      sx={{
                        display: "flex",
                        alignItems: "center",
                        gap: 0.8,
                        mb: 0.3,
                      }}
                    >
                      <BusinessIcon
                        sx={{
                          fontSize: 16,
                          color:
                            orgMode === "select"
                              ? ACCENT
                              : palette.TEXT_FADED,
                        }}
                      />
                      <Typography
                        sx={{
                          color: palette.TEXT_PRIMARY,
                          fontWeight: 500,
                          fontSize: "0.78rem",
                        }}
                      >
                        Existing organization
                      </Typography>
                    </Box>
                    <Typography
                      sx={{
                        color: palette.TEXT_DIM,
                        fontSize: "0.7rem",
                      }}
                    >
                      Use one of your organizations
                    </Typography>
                  </Box>
                  <Box
                    onClick={() => setOrgMode("create")}
                    sx={{
                      flex: 1,
                      p: 1.5,
                      borderRadius: 1.5,
                      border: `1px solid ${orgMode === "create" ? CARD_BORDER_ACTIVE : palette.CARD_BORDER}`,
                      bgcolor:
                        orgMode === "create" ? ACCENT_DIM : "transparent",
                      cursor: "pointer",
                      transition: "all 0.2s",
                      "&:hover": { borderColor: palette.BORDER_HOVER },
                    }}
                  >
                    <Box
                      sx={{
                        display: "flex",
                        alignItems: "center",
                        gap: 0.8,
                        mb: 0.3,
                      }}
                    >
                      <CreateNewFolderIcon
                        sx={{
                          fontSize: 16,
                          color:
                            orgMode === "create"
                              ? ACCENT
                              : palette.TEXT_FADED,
                        }}
                      />
                      <Typography
                        sx={{
                          color: palette.TEXT_PRIMARY,
                          fontWeight: 500,
                          fontSize: "0.78rem",
                        }}
                      >
                        New organization
                      </Typography>
                    </Box>
                    <Typography
                      sx={{
                        color: palette.TEXT_DIM,
                        fontSize: "0.7rem",
                      }}
                    >
                      Create a new organization
                    </Typography>
                  </Box>
                </Box>
              )}

              {orgMode === "select" && hasExistingOrgs ? (
                <>
                  <FormControl fullWidth size="small" sx={{ mb: 2.5 }}>
                    <InputLabel
                      sx={{
                        color: palette.TEXT_FADED,
                        fontSize: "0.82rem",
                        "&.Mui-focused": { color: ACCENT },
                      }}
                    >
                      Organization
                    </InputLabel>
                    <Select
                      value={selectedOrgId}
                      onChange={(e) => setSelectedOrgId(e.target.value)}
                      label="Organization"
                      sx={{
                        color: palette.TEXT_PRIMARY,
                        fontSize: "0.82rem",
                        "& fieldset": { borderColor: palette.INPUT_BORDER },
                        "&:hover fieldset": {
                          borderColor: palette.INPUT_BORDER_HOVER,
                        },
                        "&.Mui-focused fieldset": { borderColor: ACCENT },
                        "& .MuiSvgIcon-root": {
                          color: palette.TEXT_FADED,
                        },
                      }}
                      MenuProps={{
                        PaperProps: {
                          sx: {
                            bgcolor: palette.MENU_BG,
                            color: palette.MENU_TEXT,
                            maxHeight: 280,
                          },
                        },
                      }}
                    >
                      {existingOrgs.map((org) => (
                        <MenuItem
                          key={org.id}
                          value={org.id}
                          sx={{ fontSize: "0.82rem" }}
                        >
                          {org.display_name || org.name}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <Button
                    variant="contained"
                    onClick={handleSelectExistingOrg}
                    disabled={!selectedOrgId}
                    sx={btnSx}
                    startIcon={<BusinessIcon sx={{ fontSize: 16 }} />}
                  >
                    Continue with this organization
                  </Button>
                </>
              ) : (
                <>
                  <TextField
                    fullWidth
                    size="small"
                    label="Organization name"
                    placeholder="My Company"
                    value={orgDisplayName}
                    onChange={(e) => setOrgDisplayName(e.target.value)}
                    variant="outlined"
                    sx={{ mb: 2.5 }}
                    InputProps={{ sx: palette.inputSx }}
                    InputLabelProps={{ sx: palette.labelSx }}
                  />
                  <Button
                    variant="contained"
                    onClick={handleCreateOrg}
                    disabled={
                      createOrgMutation.isPending || !orgDisplayName.trim()
                    }
                    sx={btnSx}
                    startIcon={
                      createOrgMutation.isPending ? (
                        <CircularProgress size={14} sx={{ color: "#000" }} />
                      ) : (
                        <BusinessIcon sx={{ fontSize: 16 }} />
                      )
                    }
                  >
                    {createOrgMutation.isPending
                      ? "Creating..."
                      : "Create organization"}
                  </Button>
                </>
              )}
            </Box>
          </Fade>
        );

      case "subscription":
        return (
          <Fade in={isStepActive(stepIndex)} timeout={400}>
            <Box sx={{ mt: 2.5 }}>
              <Box
                sx={{
                  p: 2.5,
                  borderRadius: 1.5,
                  border: `1px solid ${palette.CARD_BORDER}`,
                  bgcolor: palette.CARD_BG,
                  mb: 2.5,
                }}
              >
                <Typography
                  sx={{
                    color: palette.TEXT_PRIMARY,
                    fontWeight: 600,
                    fontSize: "0.9rem",
                    mb: 1,
                  }}
                >
                  Helix Business Subscription
                </Typography>
                <Typography
                  sx={{
                    color: palette.TEXT_MUTED,
                    fontSize: "0.8rem",
                    mb: 1.5,
                  }}
                >
                  {isTrialing
                    ? "Your free trial is active. No payment method required - you have full access for the duration of the trial. Click Continue to proceed."
                    : isSubscriptionActive
                      ? "Your subscription is active. Click Continue to proceed."
                      : "Subscribe to activate your organization and unlock everything Helix offers."}
                </Typography>
                {isSubscriptionActive && wallet ? (
                  <Box sx={{ mb: 2 }}>
                    <Typography
                      sx={{
                        color: palette.TEXT_DIM,
                        fontSize: "0.75rem",
                        mb: 0.5,
                      }}
                    >
                      Status: {wallet?.subscription_status || "not_subscribed"}
                    </Typography>
                    <Typography
                      sx={{
                        color: palette.TEXT_DIM,
                        fontSize: "0.75rem",
                        mb: 0.5,
                      }}
                    >
                      Subscription started:{" "}
                      {formatUnixTimestamp(wallet.subscription_created)}
                    </Typography>
                    <Typography
                      sx={{
                        color: palette.TEXT_DIM,
                        fontSize: "0.75rem",
                        mb: 0.5,
                      }}
                    >
                      Current billing period started:{" "}
                      {formatUnixTimestamp(
                        wallet.subscription_current_period_start,
                      )}
                    </Typography>
                    <Typography
                      sx={{
                        color: palette.TEXT_DIM,
                        fontSize: "0.75rem",
                        mb: 0.5,
                      }}
                    >
                      Next billing term:{" "}
                      {formatUnixTimestamp(
                        wallet.subscription_current_period_end,
                      )}
                    </Typography>
                    <Typography
                      sx={{
                        color: palette.TEXT_DIM,
                        fontSize: "0.75rem",
                      }}
                    >
                      Current balance: ${wallet.balance?.toFixed(2) || "0.00"}{" "}
                      credits
                    </Typography>
                  </Box>
                ) : (
                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                    {SUBSCRIPTION_BENEFITS.map((benefit) => (
                      <Box
                        key={benefit}
                        sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}
                      >
                        <CheckCircleIcon
                          sx={{
                            color: ACCENT,
                            fontSize: 16,
                            mt: "2px",
                            flexShrink: 0,
                          }}
                        />
                        <Typography
                          sx={{
                            color: palette.TEXT_SECONDARY,
                            fontSize: "0.8rem",
                            lineHeight: 1.5,
                          }}
                        >
                          {benefit}
                        </Typography>
                      </Box>
                    ))}
                  </Box>
                )}
              </Box>

              <Box sx={{ display: "flex", gap: 1.5, alignItems: "center" }}>
                {isSubscriptionActive ? (
                  <Button
                    variant="contained"
                    onClick={() => markStepCompleteByType("subscription")}
                    sx={btnSx}
                  >
                    Continue
                  </Button>
                ) : (
                  <Button
                    variant="contained"
                    onClick={handleSubscribe}
                    disabled={isSubscribing}
                    sx={btnSx}
                    startIcon={
                      isSubscribing ? (
                        <CircularProgress size={14} sx={{ color: "#000" }} />
                      ) : (
                        <CreditCardIcon sx={{ fontSize: 16 }} />
                      )
                    }
                  >
                    {isSubscribing
                      ? "Redirecting to payment..."
                      : "Start Subscription ($499/m)"}
                  </Button>
                )}
                <Button
                  variant="text"
                  onClick={() => refetchWallet()}
                  disabled={isFetchingWallet}
                  sx={{
                    color: palette.TEXT_DIM,
                    textTransform: "none",
                    fontSize: "0.78rem",
                    "&:hover": { color: palette.TEXT_SECONDARY },
                  }}
                >
                  {isFetchingWallet ? "Refreshing..." : "Refresh status"}
                </Button>
              </Box>
            </Box>
          </Fade>
        );

      case "provider": {
        const inventoryLoading = providersLoading || harnessesLoading;
        const canFinish =
          !inventoryLoading && !!createdOrg?.viewer_is_owner && (
            (codingAccessOption === "helix" && helixDefaultAvailable) ||
            (codingAccessOption === "claude" && hasClaudeSubscription && !!claudeModel) ||
            (codingAccessOption === "codex" && hasCodexSubscription && !!codexModel)
          );
        const continueLabel =
          codingAccessOption === "claude"
            ? "Continue with Claude subscription"
            : codingAccessOption === "codex"
              ? "Continue with ChatGPT subscription"
              : "Continue with Helix credits";

        const codingOptions: Array<{
          id: CodingAccessOption;
          title: string;
          description: string;
          connected?: boolean;
        }> = [
          {
            id: "helix",
            title: "Helix Providers",
            description:
              "Use Helix credits for coding models. No external account required.",
          },
          {
            id: "claude",
            title: "Claude Subscription",
            description: "Use the tokens included with your Claude Pro or Max plan.",
            connected: hasClaudeSubscription,
          },
          {
            id: "codex",
            title: "ChatGPT Subscription",
            description: "Use the Codex tokens included with your ChatGPT plan.",
            connected: hasCodexSubscription,
          },
        ];

        return (
          <Fade in={isStepActive(stepIndex)} timeout={400}>
            <Box sx={{ mt: 2.5 }}>
              <Typography
                sx={{
                  color: palette.TEXT_SECONDARY,
                  fontSize: "0.78rem",
                  mb: 2,
                }}
              >
                External subscriptions are optional. Helix Providers is selected
                by default and charges model usage to your Helix credit balance.
              </Typography>
              {wallet && (
                <Typography
                  sx={{
                    color: palette.TEXT_SECONDARY,
                    fontSize: "0.78rem",
                    mb: 2,
                  }}
                >
                  You have {wallet.balance?.toFixed(2) || "0.00"} Helix credits. Helix credits pay for AI model usage in Helix.
                </Typography>
              )}
              {codingAccessOption === "helix" && !inventoryLoading && helixProvider && helixModel && !helixDefaultAvailable && (
                <Typography color="error" sx={{ fontSize: "0.78rem", mb: 2 }}>
                  The selected Helix model is not available for Zed Agent in this organization.
                </Typography>
              )}
              {!inventoryLoading && !createdOrg?.viewer_is_owner && (
                <Typography color="error" sx={{ fontSize: "0.78rem", mb: 2 }}>
                  Ask an organization owner to set the Default Runtime.
                </Typography>
              )}

              <Box
                sx={{
                  display: "grid",
                  gridTemplateColumns: {
                    xs: "1fr",
                    sm: "repeat(3, minmax(0, 1fr))",
                  },
                  gap: 1.25,
                  mb: 2,
                }}
              >
                {codingOptions.map((option) => {
                  const selected = codingAccessOption === option.id;
                  return (
                    <ButtonBase
                      key={option.id}
                      onClick={() => setCodingAccessOption(option.id)}
                      sx={{
                        display: "block",
                        textAlign: "left",
                        p: 1.5,
                        minHeight: 126,
                        borderRadius: 1.5,
                        border: `1px solid ${
                          selected ? CARD_BORDER_ACTIVE : palette.CARD_BORDER
                        }`,
                        bgcolor: selected ? ACCENT_DIM : "transparent",
                        transition: "all 0.2s",
                        "&:hover": { borderColor: palette.BORDER_HOVER },
                      }}
                    >
                      <Box
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          gap: 0.8,
                          mb: 0.75,
                        }}
                      >
                        {option.id === "helix" ? (
                          <Server
                            size={18}
                            color={selected ? ACCENT : palette.TEXT_FADED}
                          />
                        ) : option.id === "claude" ? (
                          <AnthropicLogo
                            style={{
                              width: 18,
                              height: 18,
                              color: "#d97757",
                            }}
                          />
                        ) : (
                          <AgentHarness
                            runtime="codex_cli"
                            variant="short"
                            size={18}
                            showTooltip={false}
                          />
                        )}
                        <Typography
                          sx={{
                            color: palette.TEXT_PRIMARY,
                            fontWeight: 600,
                            fontSize: "0.78rem",
                          }}
                        >
                          {option.title}
                        </Typography>
                      </Box>
                      <Typography
                        sx={{
                          color: palette.TEXT_FADED,
                          fontSize: "0.68rem",
                          lineHeight: 1.45,
                        }}
                      >
                        {option.description}
                      </Typography>
                      {option.id !== "helix" && (
                        <Typography
                          sx={{
                            color: option.connected
                              ? ACCENT
                              : palette.TEXT_DIM,
                            fontSize: "0.65rem",
                            fontWeight: 600,
                            mt: 0.75,
                          }}
                        >
                          {option.connected ? "Connected" : "Not connected"}
                        </Typography>
                      )}
                    </ButtonBase>
                  );
                })}
              </Box>

              {codingAccessOption === "helix" && (
                <Stack spacing={2} sx={{ mb: 2 }}>
                  {hasConfiguredHelixDefault ? (
                    <Box
                      sx={{
                        p: 1.5,
                        borderRadius: 1.5,
                        border: `1px solid ${palette.BORDER_SUBTLE}`,
                        bgcolor: palette.OVERLAY_FAINT,
                      }}
                    >
                      <Typography sx={{ color: palette.TEXT_PRIMARY, fontSize: "0.78rem", fontWeight: 600 }}>
                        Recommended model: {helixModel}
                      </Typography>
                      <Typography sx={{ color: palette.TEXT_FADED, fontSize: "0.68rem", mt: 0.5 }}>
                        Helix has selected the provider and reasoning settings for you.
                      </Typography>
                    </Box>
                  ) : (
                    <>
                  <FormControl fullWidth>
                    <InputLabel id="onboarding-helix-provider-label">Helix provider</InputLabel>
                    <Select
                      labelId="onboarding-helix-provider-label"
                      label="Helix provider"
                      value={helixProvider}
                      onChange={(event) => {
                        setHelixProvider(event.target.value)
                        setHelixModel("")
                      }}
                    >
                      <MenuItem value="" disabled>Select a provider</MenuItem>
                      {helixProviders.map((provider) => (
                        <MenuItem key={providerRef(provider)} value={providerRef(provider)}>
                          {provider.name || provider.id}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <FormControl fullWidth disabled={!helixProvider}>
                    <InputLabel id="onboarding-helix-model-label">Helix model</InputLabel>
                    <Select
                      labelId="onboarding-helix-model-label"
                      label="Helix model"
                      value={helixModel}
                      onChange={(event) => setHelixModel(event.target.value)}
                    >
                      <MenuItem value="" disabled>Select a model</MenuItem>
                      {helixModels.map((model) => (
                        <MenuItem key={model.id} value={model.id}>{model.id}</MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <FormControl fullWidth>
                    <InputLabel id="onboarding-helix-reasoning-label">Reasoning effort</InputLabel>
                    <Select
                      labelId="onboarding-helix-reasoning-label"
                      label="Reasoning effort"
                      value={helixReasoningEffort}
                      onChange={(event) => setHelixReasoningEffort(event.target.value)}
                    >
                      <MenuItem value="none">None</MenuItem>
                      <MenuItem value="low">Low</MenuItem>
                      <MenuItem value="medium">Medium</MenuItem>
                      <MenuItem value="high">High</MenuItem>
                    </Select>
                  </FormControl>
                    </>
                  )}
                </Stack>
              )}

              {codingAccessOption === "claude" && !hasClaudeSubscription && (
                <Box
                  sx={{
                    p: 1.5,
                    mb: 2,
                    borderRadius: 1.5,
                    border: `1px solid ${palette.BORDER_SUBTLE}`,
                    bgcolor: palette.OVERLAY_FAINT,
                  }}
                >
                  <Typography
                    sx={{
                      color: palette.TEXT_SECONDARY,
                      fontSize: "0.75rem",
                      mb: 1,
                    }}
                  >
                    Connect your personal Claude subscription before continuing.
                  </Typography>
                  <ClaudeSubscriptionConnect variant="button" />
                </Box>
              )}

              {codingAccessOption === "claude" && hasClaudeSubscription && (
                <FormControl fullWidth sx={{ mb: 2 }}>
                  <InputLabel id="onboarding-claude-model-label">Claude model</InputLabel>
                  <Select
                    labelId="onboarding-claude-model-label"
                    label="Claude model"
                    value={claudeModel}
                    onChange={(event) => setClaudeModel(event.target.value)}
                  >
                    <MenuItem value="" disabled>Select a model</MenuItem>
                    {CLAUDE_SUBSCRIPTION_MODELS.map((model) => (
                      <MenuItem key={model.id} value={model.id}>{model.label}</MenuItem>
                    ))}
                  </Select>
                </FormControl>
              )}

              {codingAccessOption === "codex" && !hasCodexSubscription && (
                <Box
                  sx={{
                    p: 1.5,
                    mb: 2,
                    borderRadius: 1.5,
                    border: `1px solid ${palette.BORDER_SUBTLE}`,
                    bgcolor: palette.OVERLAY_FAINT,
                  }}
                >
                  <Typography
                    sx={{
                      color: palette.TEXT_SECONDARY,
                      fontSize: "0.75rem",
                      mb: 1,
                    }}
                  >
                    Connect your personal ChatGPT subscription before continuing.
                  </Typography>
                  <CodexSubscriptionConnect />
                </Box>
              )}

              {codingAccessOption === "codex" && hasCodexSubscription && (
                <FormControl fullWidth sx={{ mb: 2 }}>
                  <InputLabel id="onboarding-codex-model-label">Codex model</InputLabel>
                  <Select
                    labelId="onboarding-codex-model-label"
                    label="Codex model"
                    value={codexModel}
                    onChange={(event) => setCodexModel(event.target.value)}
                  >
                    <MenuItem value="" disabled>Select a model</MenuItem>
                    {CODEX_SUBSCRIPTION_MODELS.map((model) => (
                      <MenuItem key={model.id} value={model.id}>{model.label}</MenuItem>
                    ))}
                  </Select>
                </FormControl>
              )}

              <Button
                variant="contained"
                onClick={handleComplete}
                disabled={!canFinish || finishingOnboarding}
                sx={btnSx}
                startIcon={
                  finishingOnboarding ? (
                    <CircularProgress size={14} sx={{ color: "#000" }} />
                  ) : undefined
                }
              >
                {finishingOnboarding ? "Finishing setup..." : continueLabel}
              </Button>
            </Box>
          </Fade>
        );
      }
      default:
        return null;
    }
  };

  return (
    <Box
      sx={{
        position: "fixed",
        inset: 0,
        bgcolor: palette.BG,
        color: palette.TEXT_PRIMARY,
        zIndex: 1300,
        display: "flex",
        justifyContent: "center",
        alignItems: "flex-start",
        overflowY: "auto",
        pt: { xs: 4, md: 6 },
        pb: 6,
      }}
    >
      {/* Dismiss button */}
      <IconButton
        onClick={handleDismiss}
        sx={{
          position: "fixed",
          top: 16,
          right: 16,
          color: palette.TEXT_DIM,
          "&:hover": { color: palette.TEXT_SECONDARY },
          zIndex: 1301,
        }}
      >
        <CloseIcon />
      </IconButton>
      <Box
        sx={{
          width: "100%",
          maxWidth: 580,
          px: { xs: 2, md: 0 },
        }}
      >
        {/* Header */}
        <Fade in timeout={600}>
          <Box sx={{ mb: 5 }}>
            <Typography
              sx={{
                color: palette.TEXT_PRIMARY,
                fontWeight: 700,
                mb: 0.5,
                fontSize: { xs: "1.5rem", md: "1.8rem" },
                letterSpacing: "-0.02em",
              }}
            >
              Hello, {userName}
            </Typography>
            <Typography
              sx={{
                color: palette.TEXT_FADED,
                fontSize: "0.88rem",
              }}
            >
              Let&apos;s set you up for success 😉
            </Typography>
          </Box>
        </Fade>

        {/* Steps */}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
          {visibleSteps.map((step, index) => {
            const completed = isStepCompleted(index);
            const active = isStepActive(index);
            const locked = isStepLocked(index);
            const stepSubtitle =
              step.type === "organization" && createdOrg
                ? `Selected organization: ${createdOrg.display_name || createdOrg.name}`
                : step.type === "subscription" && isSubscriptionActive
                  ? isTrialing
                    ? "Free trial is active - no payment method required."
                    : "Subscription is active."
                  : step.subtitle;
            const stepTitle =
              step.type === "subscription" && isSubscriptionActive
                ? isTrialing
                  ? "Trial active"
                  : "Subscription active"
                : step.title;

            return (
              <Fade in timeout={600 + index * 150} key={index}>
                <Box
                  sx={{
                    px: { xs: 2.5, md: 3 },
                    py: { xs: 2, md: 2.5 },
                    borderRadius: 2,
                    border: `1px solid ${active ? CARD_BORDER_ACTIVE : palette.CARD_BORDER}`,
                    bgcolor: active
                      ? palette.CARD_BG_ACTIVE
                      : completed
                        ? palette.CARD_BG
                        : "transparent",
                    transition: "all 0.3s ease",
                    opacity: locked ? 0.35 : 1,
                    ...(active && {
                      boxShadow: `0 0 24px rgba(0, 232, 145, 0.04)`,
                    }),
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                    {renderStepIcon(index)}
                    <Box sx={{ flex: 1 }}>
                      <Typography
                        sx={{
                          color:
                            completed || active
                              ? palette.TEXT_PRIMARY
                              : palette.TEXT_FADED,
                          fontWeight: 600,
                          fontSize: "0.88rem",
                        }}
                      >
                        {stepTitle}
                      </Typography>
                      <Typography
                        sx={{
                          color: completed || active ? palette.TEXT_SECONDARY : palette.TEXT_DIM,
                          fontSize: "0.76rem",
                          mt: 0.2,
                        }}
                      >
                        {stepSubtitle}
                      </Typography>
                    </Box>
                  </Box>

                  {active && step.type !== "signin" && renderStepContent(index)}
                </Box>
              </Fade>
            );
          })}
        </Box>

      </Box>
    </Box>
  );
}
