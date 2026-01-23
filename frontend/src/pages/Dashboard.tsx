import ClearIcon from "@mui/icons-material/Clear";
import StorageIcon from "@mui/icons-material/Storage";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import Divider from "@mui/material/Divider";
import FormControl from "@mui/material/FormControl";
import FormControlLabel from "@mui/material/FormControlLabel";
import FormGroup from "@mui/material/FormGroup";
import Grid from "@mui/material/Grid";
import IconButton from "@mui/material/IconButton";
import InputLabel from "@mui/material/InputLabel";
import MenuItem from "@mui/material/MenuItem";
import Paper from "@mui/material/Paper/Paper";
import Select from "@mui/material/Select";
import Switch from "@mui/material/Switch";
import Tabs from "@mui/material/Tabs";
import Tab from "@mui/material/Tab";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import React, { FC, useCallback, useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import LLMCallsTable from "../components/dashboard/LLMCallsTable";
import Interaction from "../components/session/Interaction";
import RunnerSummary from "../components/session/RunnerSummary";
import SessionBadgeKey from "../components/session/SessionBadgeKey";
import SessionToolbar from "../components/session/SessionToolbar";
import SessionSummary from "../components/session/SessionSummary";
import Page from "../components/system/Page";
import JsonWindowLink from "../components/widgets/JsonWindowLink";
import Window from "../components/widgets/Window";
import useAccount from "../hooks/useAccount";
import useApi from "../hooks/useApi";
import { useGetDashboardData } from "../services/dashboardService";
import useRouter from "../hooks/useRouter";
import { ISessionSummary } from "../types";
import {
    TypesInteraction,
    TypesSession,
    TypesSessionSummary,
} from "../api/api";

import { TypesWorkloadSummary, TypesDashboardRunner } from "../api/api";
import ProviderEndpointsTable from "../components/dashboard/ProviderEndpointsTable";
import OAuthProvidersTable from "../components/dashboard/OAuthProvidersTable";
import HelixModelsTable from "../components/dashboard/HelixModelsTable";
import SchedulingDecisionsTable from "../components/dashboard/SchedulingDecisionsTable";
import GlobalSchedulingVisualization from "../components/dashboard/GlobalSchedulingVisualization";
import SystemSettingsTable from "../components/dashboard/SystemSettingsTable";
import ServiceConnectionsTable from "../components/dashboard/ServiceConnectionsTable";
import AgentSandboxes from "../components/admin/AgentSandboxes";
import UsersTable from "../components/dashboard/UsersTable";
import Chip from "@mui/material/Chip";
import { useFloatingRunnerState } from "../contexts/floatingRunnerState";
import LaunchIcon from "@mui/icons-material/Launch";
import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import SchedulerHealthIndicators from "../components/dashboard/SchedulerHealthIndicators";

const START_ACTIVE = true;

const Dashboard: FC = () => {
    const account = useAccount();
    const router = useRouter();
    const api = useApi();
    const floatingRunnerState = useFloatingRunnerState();

    const activeRef = useRef(START_ACTIVE);

    const [viewingSession, setViewingSession] = useState<TypesSession>();
    const [active, setActive] = useState(START_ACTIVE);
    const [sessionFilter, setSessionFilter] = useState("");
    const [selectedSandboxId, setSelectedSandboxId] = useState<string>("");
    const apiClient = api.getApiClient();

    const { session_id, tab, filter_sessions } = router.params;

    // Fetch list of registered sandbox instances for the agent_sandboxes tab
    const { data: sandboxInstances, isLoading: isLoadingSandboxes } = useQuery({
        queryKey: ["sandbox-instances"],
        queryFn: async () => {
            const response = await apiClient.v1SandboxesList();
            return response.data;
        },
        enabled: tab === "agent_sandboxes",
        refetchInterval: 10000,
    });

    // Don't auto-select - default to "All Sandboxes" view for aggregate monitoring

    const onViewSession = useCallback((session_id: string) => {
        router.setParams({
            session_id,
        });
    }, []);

    const onCloseViewingSession = useCallback(() => {
        setViewingSession(undefined);
        router.removeParams(["session_id"]);
    }, []);

    useEffect(() => {
        if (!session_id) return;
        if (!account.user) return;
        const loadSession = async () => {
            const session = await api.get<TypesSession>(
                `/api/v1/sessions/${session_id}`,
            );
            if (!session) return;
            setViewingSession(session);
        };
        loadSession();
    }, [account.user, session_id]);

    const { data: dashboardData, isLoading: isLoadingDashboardData } =
        useGetDashboardData();

    useEffect(() => {
        if (filter_sessions) {
            setSessionFilter(filter_sessions);
        }
    }, [filter_sessions]);

    const handleFilterChange = (event: React.ChangeEvent<HTMLInputElement>) => {
        const newFilter = event.target.value;
        setSessionFilter(newFilter);
        if (newFilter) {
            router.setParams({ filter_sessions: newFilter });
        } else {
            router.removeParams(["filter_sessions"]);
        }
    };

    const clearFilter = () => {
        setSessionFilter("");
        router.removeParams(["filter_sessions"]);
    };

    if (!account.user) return null;
    if (isLoadingDashboardData) return null;

    return (
        <Page
            breadcrumbTitle="Admin Panel"
            topbarContent={
                <Box
                    sx={{
                        width: "100%",
                        display: "flex",
                        flexDirection: "row",
                        alignItems: "center",
                        justifyContent: "space-between",
                    }}
                >
                    <Box sx={{ flex: 1 }}></Box>
                    {account.serverConfig.version && (
                        <Typography
                            variant="body2"
                            sx={{
                                color: "rgba(255, 255, 255, 0.7)",
                                position: "absolute",
                                left: "50%",
                                transform: "translateX(-50%)",
                                textAlign: "center",
                            }}
                        >
                            Helix Control Plane version:{" "}
                            {account.serverConfig.version}
                        </Typography>
                    )}
                    <Box
                        sx={{
                            flex: 1,
                            display: "flex",
                            justifyContent: "flex-end",
                        }}
                    >
                        {tab === "runners" && <SessionBadgeKey />}
                    </Box>
                </Box>
            }
        >
            <Container
                maxWidth="xl"
                sx={{
                    mt: 4,
                    height: "100%",
                    display: "flex",
                    flexDirection: "column",
                }}
            >
                {(!tab || tab === "llm_calls") && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <Box
                            sx={{
                                mb: 2,
                                display: "flex",
                                alignItems: "center",
                            }}
                        >
                            <TextField
                                label="Filter by Session ID"
                                variant="outlined"
                                value={sessionFilter}
                                onChange={handleFilterChange}
                                sx={{ flexGrow: 1, mr: 1 }}
                            />
                            {sessionFilter && (
                                <IconButton onClick={clearFilter} size="small">
                                    <ClearIcon />
                                </IconButton>
                            )}
                        </Box>
                        <LLMCallsTable sessionFilter={sessionFilter} />
                    </Box>
                )}

                {tab === "providers" && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <ProviderEndpointsTable />
                    </Box>
                )}

                {tab === "oauth_providers" && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <OAuthProvidersTable />
                    </Box>
                )}

                {tab === "service_connections" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <ServiceConnectionsTable />
                    </Box>
                )}

                {tab === "runners" && (
                    <Box
                        sx={{
                            width: "100%",
                            display: "flex",
                            flexDirection: "column",
                            alignItems: "flex-start",
                            justifyContent: "flex-start",
                        }}
                    >
                        {/* Controls for entire Runners tab */}
                        <Box
                            sx={{
                                width: "100%",
                                display: "flex",
                                flexDirection: "row",
                                alignItems: "center",
                                justifyContent: "space-between",
                                mb: 3,
                                px: 2,
                            }}
                        >
                            <Box
                                sx={{
                                    display: "flex",
                                    alignItems: "center",
                                    gap: 2,
                                }}
                            >
                                <FormGroup>
                                    <FormControlLabel
                                        control={
                                            <Switch
                                                checked={active}
                                                onChange={(
                                                    event: React.ChangeEvent<HTMLInputElement>,
                                                ) => {
                                                    activeRef.current =
                                                        event.target.checked;
                                                    setActive(
                                                        event.target.checked,
                                                    );
                                                }}
                                            />
                                        }
                                        label="Live Updates?"
                                    />
                                </FormGroup>

                                {account.admin && (
                                    <Tooltip
                                        title="Toggle floating runner state view (Ctrl/Cmd+Shift+S)"
                                        arrow
                                    >
                                        <Button
                                            variant="outlined"
                                            size="small"
                                            startIcon={<LaunchIcon />}
                                            onClick={(e) => {
                                                const rect =
                                                    e.currentTarget.getBoundingClientRect();
                                                const clickPosition = {
                                                    x: rect.left - 100, // Position floating window to the left of button
                                                    y: rect.top - 50, // Position slightly above the button
                                                };
                                                floatingRunnerState.toggleFloatingRunnerState(
                                                    clickPosition,
                                                );
                                            }}
                                            sx={{
                                                borderColor:
                                                    "rgba(0, 200, 255, 0.3)",
                                                color: floatingRunnerState.isVisible
                                                    ? "#00c8ff"
                                                    : "rgba(255, 255, 255, 0.7)",
                                                backgroundColor:
                                                    floatingRunnerState.isVisible
                                                        ? "rgba(0, 200, 255, 0.1)"
                                                        : "transparent",
                                                "&:hover": {
                                                    borderColor:
                                                        "rgba(0, 200, 255, 0.5)",
                                                    backgroundColor:
                                                        "rgba(0, 200, 255, 0.1)",
                                                },
                                            }}
                                        >
                                            {floatingRunnerState.isVisible
                                                ? "Hide"
                                                : "Show"}{" "}
                                            Floating View
                                        </Button>
                                    </Tooltip>
                                )}
                            </Box>

                            <JsonWindowLink data={dashboardData}>
                                view data
                            </JsonWindowLink>
                        </Box>

                        <Box
                            sx={{
                                width: "100%",
                                display: "flex",
                                flexDirection: "row",
                                alignItems: "flex-start",
                                justifyContent: "flex-start",
                            }}
                        >
                            {/* Queue Section */}
                            <Box
                                sx={{
                                    p: 3,
                                    flexGrow: 0,
                                    width: "480px",
                                    minWidth: "480px",
                                    overflowY: "auto",
                                    display: { xs: "none", md: "block" },
                                    borderRight:
                                        "1px solid rgba(255, 255, 255, 0.08)",
                                }}
                            >
                                {/* Queue Section Header */}
                                <Box
                                    sx={{
                                        mb: 3,
                                        pb: 1,
                                        borderBottom:
                                            "1px solid rgba(255, 255, 255, 0.1)",
                                    }}
                                >
                                    <Typography
                                        variant="h5"
                                        sx={{
                                            color: "rgba(255, 255, 255, 0.95)",
                                            fontWeight: 600,
                                            display: "flex",
                                            alignItems: "center",
                                            gap: 2,
                                        }}
                                    >
                                        Queue
                                        {dashboardData &&
                                            dashboardData.queue &&
                                            dashboardData.queue.length > 0 && (
                                                <Chip
                                                    size="small"
                                                    label={
                                                        dashboardData.queue
                                                            .length
                                                    }
                                                    sx={{
                                                        height: 22,
                                                        minWidth: 20,
                                                        backgroundColor:
                                                            "rgba(128, 90, 213, 0.15)",
                                                        color: "rgba(255, 255, 255, 0.7)",
                                                        border: "1px solid rgba(128, 90, 213, 0.3)",
                                                        "& .MuiChip-label": {
                                                            px: 1,
                                                            fontSize: "0.7rem",
                                                            fontWeight: 600,
                                                        },
                                                    }}
                                                />
                                            )}
                                    </Typography>
                                </Box>

                                {dashboardData &&
                                    dashboardData?.queue?.length === 0 && (
                                        <Box
                                            sx={{
                                                py: 4,
                                                textAlign: "center",
                                                backgroundColor:
                                                    "rgba(25, 25, 28, 0.3)",
                                                borderRadius: "3px",
                                            }}
                                        >
                                            <Typography
                                                variant="body2"
                                                sx={{
                                                    color: "rgba(255, 255, 255, 0.5)",
                                                }}
                                            >
                                                No jobs in queue
                                            </Typography>
                                        </Box>
                                    )}

                                {dashboardData &&
                                    dashboardData?.queue?.map(
                                        (item: TypesWorkloadSummary) => {
                                            return (
                                                <SessionSummary
                                                    key={item.id}
                                                    session={
                                                        {
                                                            session_id: item.id,
                                                            created:
                                                                item.created,
                                                            updated:
                                                                item.updated,
                                                            model_name:
                                                                item.model_name,
                                                            mode: item.mode,
                                                            type: item.runtime,
                                                            owner: "todo",
                                                            lora_dir:
                                                                item.lora_dir,
                                                            summary:
                                                                item.summary,
                                                            app_id: "todo",
                                                        } as ISessionSummary
                                                    }
                                                    onViewSession={
                                                        onViewSession
                                                    }
                                                    hideViewButton={true}
                                                />
                                            );
                                        },
                                    )}
                            </Box>

                            {/* Runners Section */}
                            <Box
                                sx={{
                                    flexGrow: 1,
                                    p: 3,
                                    height: "100%",
                                    width: "100%",
                                    overflowY: "auto",
                                }}
                            >
                                {/* Runners Section Header */}
                                <Box
                                    sx={{
                                        display: "flex",
                                        alignItems: "center",
                                        justifyContent: "space-between",
                                        mb: 4,
                                    }}
                                >
                                    <Typography
                                        variant="h5"
                                        sx={{
                                            color: "rgba(255, 255, 255, 0.95)",
                                            fontWeight: 600,
                                        }}
                                    >
                                        Runner State
                                    </Typography>
                                    <Box
                                        sx={{
                                            display: "flex",
                                            alignItems: "center",
                                            gap: 2,
                                        }}
                                    >
                                        <Typography
                                            variant="caption"
                                            sx={{
                                                color: "rgba(255, 255, 255, 0.7)",
                                                fontSize: "0.75rem",
                                            }}
                                        >
                                            Scheduler Health:
                                        </Typography>
                                        <SchedulerHealthIndicators runnerId="" />
                                    </Box>
                                </Box>
                                <Box
                                    sx={{
                                        borderBottom:
                                            "1px solid rgba(255, 255, 255, 0.1)",
                                        mb: 4,
                                    }}
                                />

                                <Grid
                                    container
                                    sx={{
                                        width: "100%",
                                        overflow: "auto",
                                    }}
                                    spacing={3}
                                >
                                    {dashboardData &&
                                        dashboardData?.runners?.length ===
                                            0 && (
                                            <Grid item xs={12}>
                                                <Box
                                                    sx={{
                                                        py: 4,
                                                        textAlign: "center",
                                                        backgroundColor:
                                                            "rgba(25, 25, 28, 0.3)",
                                                        borderRadius: "3px",
                                                    }}
                                                >
                                                    <Typography
                                                        variant="body2"
                                                        sx={{
                                                            color: "rgba(255, 255, 255, 0.5)",
                                                        }}
                                                    >
                                                        No active runners
                                                    </Typography>
                                                </Box>
                                            </Grid>
                                        )}

                                    {dashboardData &&
                                        dashboardData?.runners?.map(
                                            (runner: TypesDashboardRunner) => {
                                                return (
                                                    <Grid
                                                        item
                                                        xs={12}
                                                        key={runner.id}
                                                    >
                                                        <RunnerSummary
                                                            runner={runner}
                                                            onViewSession={
                                                                onViewSession
                                                            }
                                                        />
                                                    </Grid>
                                                );
                                            },
                                        )}
                                </Grid>

                                {/* Global Scheduling Visualization Section */}
                                <Box sx={{ mt: 4 }}>
                                    <GlobalSchedulingVisualization
                                        decisions={
                                            (dashboardData as any)
                                                ?.global_allocation_decisions ||
                                            []
                                        }
                                    />
                                </Box>

                                {/* Scheduling Decisions Section */}
                                <Box sx={{ mt: 4 }}>
                                    <Typography
                                        variant="h5"
                                        sx={{
                                            mb: 3,
                                            color: "rgba(255, 255, 255, 0.95)",
                                            fontWeight: 600,
                                            borderBottom:
                                                "1px solid rgba(255, 255, 255, 0.1)",
                                            pb: 1,
                                        }}
                                    >
                                        Recent Scheduling Decisions
                                    </Typography>

                                    <Box
                                        sx={{
                                            mb: 2,
                                            color: "rgba(255, 255, 255, 0.6)",
                                            fontSize: "0.875rem",
                                        }}
                                    >
                                        Live log of decisions made by the
                                        central scheduler when assigning
                                        workloads to runners
                                    </Box>

                                    <SchedulingDecisionsTable
                                        decisions={
                                            dashboardData?.scheduling_decisions ||
                                            []
                                        }
                                    />
                                </Box>
                            </Box>
                        </Box>
                    </Box>
                )}

                {tab === "helix_models" && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <HelixModelsTable />
                    </Box>
                )}

                {tab === "system_settings" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <SystemSettingsTable />
                    </Box>
                )}

                {tab === "agent_sandboxes" && account.admin && (
                    <Box sx={{ width: "100%" }}>
                        {/* Sandbox Selector */}
                        <Box sx={{ mb: 3, display: "flex", alignItems: "center", gap: 2, flexWrap: "wrap" }}>
                            <FormControl size="small" sx={{ minWidth: 400 }}>
                                <InputLabel id="sandbox-selector-label">Agent Sandbox</InputLabel>
                                <Select
                                    labelId="sandbox-selector-label"
                                    value={selectedSandboxId}
                                    label="Agent Sandbox"
                                    onChange={(e) => setSelectedSandboxId(e.target.value)}
                                    disabled={isLoadingSandboxes || !sandboxInstances?.length}
                                    startAdornment={<StorageIcon sx={{ mr: 1, color: "text.secondary" }} />}
                                >
                                    <MenuItem value="">
                                        <em>All Sandboxes</em>
                                    </MenuItem>
                                    {sandboxInstances?.map((instance) => (
                                        <MenuItem key={instance.id} value={instance.id}>
                                            {instance.gpu_vendor ? "🖥️" : "💻"}{" "}
                                            {instance.hostname || instance.id} ({instance.active_sandboxes || 0}/{instance.max_sandboxes || 10} sessions)
                                        </MenuItem>
                                    ))}
                                </Select>
                            </FormControl>
                            {sandboxInstances && sandboxInstances.length > 0 && (
                                <Typography variant="body2" color="text.secondary">
                                    {sandboxInstances.length} sandbox{sandboxInstances.length !== 1 ? "es" : ""} registered
                                </Typography>
                            )}
                            {!isLoadingSandboxes && (!sandboxInstances || sandboxInstances.length === 0) && (
                                <Typography variant="body2" color="warning.main">
                                    No agent sandboxes registered
                                </Typography>
                            )}
                        </Box>
                        <AgentSandboxes selectedSandboxId={selectedSandboxId} />
                    </Box>
                )}

                {tab === "users" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <UsersTable />
                    </Box>
                )}

                {viewingSession && (
                    <Window
                        open
                        size="lg"
                        background="#FAEFE0"
                        withCancel
                        cancelTitle="Close"
                        onCancel={onCloseViewingSession}
                    >
                        <SessionToolbar session={viewingSession} />
                        {viewingSession.interactions?.map(
                            (interaction: TypesInteraction, i: number) => {
                                return (
                                    <Interaction
                                        key={i}
                                        serverConfig={account.serverConfig}
                                        interaction={interaction}
                                        session={viewingSession}
                                        onRegenerate={() => {}}
                                        onReloadSession={async () => {}}
                                        isOwner={true}
                                        isAdmin={false}
                                        session_id={viewingSession.id || ""}
                                        highlightAllFiles={false}
                                        isLastInteraction={
                                            i ===
                                            (viewingSession.interactions
                                                ?.length || 0) -
                                                1
                                        }
                                    />
                                );
                            },
                        )}
                    </Window>
                )}
            </Container>
        </Page>
    );
};

export default Dashboard;
