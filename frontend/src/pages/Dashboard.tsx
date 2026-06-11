import ClearIcon from "@mui/icons-material/Clear";
import WarningAmberIcon from "@mui/icons-material/WarningAmber";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import Divider from "@mui/material/Divider";
import FormControlLabel from "@mui/material/FormControlLabel";
import FormGroup from "@mui/material/FormGroup";
import Grid from "@mui/material/Grid";
import IconButton from "@mui/material/IconButton";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import React, { FC, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import LLMCallsTable from "../components/dashboard/LLMCallsTable";
import Interaction from "../components/session/Interaction";
import SessionToolbar from "../components/session/SessionToolbar";
import SessionSummary from "../components/session/SessionSummary";
// Page wrapper removed - Dashboard is now rendered inside FullScreenDialog
import JsonWindowLink from "../components/widgets/JsonWindowLink";
import Window from "../components/widgets/Window";
import useAccount from "../hooks/useAccount";
import useApi from "../hooks/useApi";
import { ISessionSummary } from "../types";
import {
    TypesInteraction,
    TypesSession,
    TypesSessionSummary,
} from "../api/api";

import ProviderEndpointsTable from "../components/dashboard/ProviderEndpointsTable";
import OAuthProvidersTable from "../components/dashboard/OAuthProvidersTable";
import HelixModelsTable from "../components/dashboard/HelixModelsTable";
import PricingTable from "../components/dashboard/PricingTable";
import SystemSettingsTable from "../components/dashboard/SystemSettingsTable";
import ServiceConnectionsTable from "../components/dashboard/ServiceConnectionsTable";
import Runners from "../components/admin/Runners";
import RunnerProfilesTable from "../components/dashboard/RunnerProfilesTable";
import AdminOrgsTable from "../components/dashboard/AdminOrgsTable";
import UsersTable from "../components/dashboard/UsersTable";
import UserDetailPanel from "../components/dashboard/UserDetailPanel";
import KoditAdminTable from "../components/dashboard/KoditAdminTable";
import KoditAdminRepoDetail from "../components/dashboard/KoditAdminRepoDetail";
import KoditAdminQueue from "../components/dashboard/KoditAdminQueue";
import Chip from "@mui/material/Chip";
import LaunchIcon from "@mui/icons-material/Launch";
import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";

const START_ACTIVE = true;

interface DashboardProps {
    tab?: string
    initialSessionFilter?: string
}

const Dashboard: FC<DashboardProps> = ({ tab = "llm_calls", initialSessionFilter }) => {
    const account = useAccount();
    const api = useApi();

    const activeRef = useRef(START_ACTIVE);

    const [viewingSession, setViewingSession] = useState<TypesSession>();
    const [active, setActive] = useState(START_ACTIVE);
    const [sessionFilter, setSessionFilter] = useState("");
    const [sessionIdParam, setSessionIdParam] = useState<string>("");
    const [repoId, setRepoId] = useState<string>("");
    const [selectedUserId, setSelectedUserId] = useState<string>("");
    const apiClient = api.getApiClient();

    const session_id = sessionIdParam;

    // Fetch list of registered sandbox instances for the Runners tab.
    // Used solely for the version-mismatch alert that surfaces when any
    // online runner reports a helix_version different from the control
    // plane. Tab-gated and only refetched while the tab is open.
    const { data: allSandboxInstances } = useQuery({
        queryKey: ["sandbox-instances"],
        queryFn: async () => {
            const response = await apiClient.v1SandboxesList();
            return response.data;
        },
        enabled: tab === "runners",
        refetchInterval: 10000,
    });

    const sandboxInstances = useMemo(
        () => allSandboxInstances?.filter((i) => i.status === "online"),
        [allSandboxInstances],
    );

    const onViewSession = useCallback((session_id: string) => {
        setSessionIdParam(session_id);
    }, []);

    const onCloseViewingSession = useCallback(() => {
        setViewingSession(undefined);
        setSessionIdParam("");
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

    const handleFilterChange = (event: React.ChangeEvent<HTMLInputElement>) => {
        setSessionFilter(event.target.value);
    };

    const clearFilter = () => {
        setSessionFilter("");
    };

    useEffect(() => {
        setSessionFilter(initialSessionFilter || "");
    }, [initialSessionFilter]);

    if (!account.user) return null;

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
            {/* Version info bar */}
            <Box
                sx={{
                    width: "100%",
                    display: "flex",
                    flexDirection: "row",
                    alignItems: "center",
                    justifyContent: "space-between",
                    px: 3,
                    py: 1,
                    borderBottom: '1px solid rgba(255, 255, 255, 0.06)',
                    flexShrink: 0,
                }}
            >
                <Box sx={{ flex: 1 }}></Box>
                {account.serverConfig.version && (
                    <Typography
                        variant="body2"
                        sx={{
                            color: "rgba(255, 255, 255, 0.7)",
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
                </Box>
            </Box>
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

                {tab === "helix_models" && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <HelixModelsTable />
                    </Box>
                )}

                {tab === "runner_profiles" && account.admin && (
                    <Box sx={{ width: "100%", p: 2 }}>
                        <RunnerProfilesTable />
                    </Box>
                )}

                {tab === "pricing" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <PricingTable />
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

                {tab === "runners" && account.admin && (
                    <Box sx={{ width: "100%" }}>
                        {/* Version Mismatch Alert - skip for dev builds (SHA1 hashes or unknown).
                            Kept at this layer (not inside Runners.tsx) so it's visible across
                            every sub-tab, not just Overview. */}
                        {(() => {
                            const controlPlaneVersion = account.serverConfig?.version;
                            if (!controlPlaneVersion || !sandboxInstances) return null;

                            if (controlPlaneVersion === "<unknown>") return null;
                            const isSha1Hash = /^[a-f0-9]{40}$/i.test(controlPlaneVersion);
                            if (isSha1Hash) return null;

                            const mismatched = sandboxInstances.filter(
                                (s) => s.helix_version && s.status === "online" && s.helix_version !== controlPlaneVersion
                            );
                            if (mismatched.length === 0) return null;

                            const fmt = (v: string) => v.length > 7 ? v.substring(0, 7) : v;
                            return (
                                <Alert
                                    severity="warning"
                                    icon={<WarningAmberIcon />}
                                    sx={{ m: 2, mb: 0 }}
                                >
                                    <strong>Version Mismatch:</strong> {mismatched.length} runner{mismatched.length > 1 ? "s" : ""} on a different version than control plane (v{fmt(controlPlaneVersion)}):
                                    {" "}
                                    {mismatched.map((s, i) => (
                                        <span key={s.id}>
                                            {i > 0 && ", "}
                                            <strong>{s.hostname || s.id}</strong> (v{fmt(s.helix_version || "unknown")})
                                        </span>
                                    ))}
                                </Alert>
                            );
                        })()}
                        <Runners />
                    </Box>
                )}

                {tab === "users" && account.admin && !selectedUserId && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <UsersTable onSelectUser={(id) => setSelectedUserId(id)} />
                    </Box>
                )}

                {tab === "users" && account.admin && selectedUserId && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <UserDetailPanel
                            userId={selectedUserId}
                            onBack={() => setSelectedUserId("")}
                        />
                    </Box>
                )}

                {tab === "orgs" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <AdminOrgsTable />
                    </Box>
                )}

                {tab === "kodit" && account.admin && !repoId && (
                    <Box sx={{ width: "100%", p: 2 }}>
                        <KoditAdminTable onViewDetail={(id) => setRepoId(id)} />
                    </Box>
                )}

                {tab === "kodit" && account.admin && repoId && (
                    <Box sx={{ width: "100%", p: 2 }}>
                        <KoditAdminRepoDetail koditRepoId={repoId} onBack={() => setRepoId("")} />
                    </Box>
                )}

                {tab === "kodit_queue" && account.admin && (
                    <Box sx={{ width: "100%", p: 2 }}>
                        <KoditAdminQueue />
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
        </Box>
    );
};

export default Dashboard;
