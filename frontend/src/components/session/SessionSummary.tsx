import React, { FC, useRef } from "react";
import IconButton from "@mui/material/IconButton";
import Typography from "@mui/material/Typography";
import VisibilityIcon from "@mui/icons-material/Visibility";
import SessionBadge from "./SessionBadge";
import JsonWindowLink from "../widgets/JsonWindowLink";
import Paper from "@mui/material/Paper";
import Box from "@mui/material/Box";
import Grid from "@mui/material/Grid";
import Chip from "@mui/material/Chip";
import AccessTimeIcon from "@mui/icons-material/AccessTime";

import { ISessionSummary } from "../../types";

import {
    getSummaryCaption,
    getHeadline,
    shortID,
    getTiming,
} from "../../utils/session";

export const SessionSummary: FC<{
    session: ISessionSummary;
    onViewSession: {
        (id: string): void;
    };
    hideViewButton?: boolean;
}> = ({ session, onViewSession, hideViewButton = false }) => {
    // Get basic info
    const modelHeadline = getHeadline(
        session.model_name,
        session.mode,
        session.lora_dir,
    );
    const summaryCaption = getSummaryCaption(session);
    const timing = getTiming(session);
    const id = shortID(session.session_id);

    // Reference to the hidden json window link
    const jsonLinkRef = useRef<HTMLDivElement>(null);

    // Function to show json data
    const showJsonData = () => {
        if (jsonLinkRef.current) {
            const link = jsonLinkRef.current.querySelector("a");
            if (link) link.click();
        }
    };

    return (
        <Paper
            elevation={0}
            sx={{
                width: "100%",
                my: 1.5,
                backgroundColor: "rgba(25, 25, 28, 0.6)",
                position: "relative",
                borderRadius: "3px",
                overflow: "hidden",
                boxShadow: "0 2px 8px -2px rgba(0, 0, 0, 0.15)",
                border: "1px solid rgba(255, 255, 255, 0.05)",
            }}
        >
            {/* Left accent bar */}
            <Box
                sx={{
                    position: "absolute",
                    left: 0,
                    top: 0,
                    bottom: 0,
                    width: "3px",
                    background:
                        "linear-gradient(180deg, rgba(128, 90, 213, 0.9) 0%, rgba(128, 90, 213, 0.4) 100%)",
                    boxShadow: "0 0 8px rgba(128, 90, 213, 0.3)",
                }}
            />

            <Box sx={{ p: 2, pl: 3 }}>
                <Grid container spacing={1} alignItems="center">
                    <Grid item sx={{ pr: 1 }}>
                        <Box sx={{ transform: "scale(1.1)" }}>
                            <SessionBadge modelName={session.model_name} />
                        </Box>
                    </Grid>
                    <Grid item>
                        <Typography
                            variant="subtitle1"
                            sx={{
                                color: "rgba(255, 255, 255, 0.95)",
                                fontWeight: 500,
                                fontSize: "0.9rem",
                            }}
                        >
                            {modelHeadline}
                        </Typography>
                    </Grid>
                    <Grid item>
                        <Chip
                            size="small"
                            label={id}
                            sx={{
                                height: 22,
                                backgroundColor: "rgba(128, 90, 213, 0.15)",
                                border: "1px solid rgba(128, 90, 213, 0.3)",
                                color: "rgba(255, 255, 255, 0.8)",
                                fontFamily: "monospace",
                                fontSize: "0.7rem",
                                cursor: "pointer",
                                "& .MuiChip-label": {
                                    px: 1,
                                },
                            }}
                            onClick={showJsonData}
                        />
                        {/* Hidden link for the JSON window functionality */}
                        <Box sx={{ display: "none" }} ref={jsonLinkRef}>
                            <JsonWindowLink data={session}>{id}</JsonWindowLink>
                        </Box>
                    </Grid>
                    <Grid item sx={{ flexGrow: 1 }} />
                    <Grid item>
                        <Box
                            sx={{
                                display: "flex",
                                alignItems: "center",
                                background: "rgba(255, 255, 255, 0.03)",
                                border: "1px solid rgba(255, 255, 255, 0.05)",
                                borderRadius: "3px",
                                px: 1,
                                py: 0.3,
                            }}
                        >
                            <AccessTimeIcon
                                sx={{
                                    fontSize: 14,
                                    color: "rgba(255, 255, 255, 0.6)",
                                    mr: 0.5,
                                }}
                            />
                            <Typography
                                variant="caption"
                                sx={{
                                    color: "rgba(255, 255, 255, 0.6)",
                                    fontWeight: 500,
                                    fontSize: "0.75rem",
                                    letterSpacing: "0.3px",
                                }}
                            >
                                {timing}
                            </Typography>
                        </Box>
                    </Grid>
                    {!hideViewButton && (
                        <Grid item>
                            <IconButton
                                size="small"
                                sx={{
                                    color: "rgba(255, 255, 255, 0.8)",
                                    backgroundColor: "rgba(128, 90, 213, 0.1)",
                                    border: "1px solid rgba(128, 90, 213, 0.2)",
                                    ml: 1,
                                }}
                                onClick={() => {
                                    onViewSession(session.session_id);
                                }}
                            >
                                <VisibilityIcon fontSize="small" />
                            </IconButton>
                        </Grid>
                    )}
                </Grid>

                {summaryCaption && (
                    <Typography
                        variant="body2"
                        sx={{
                            color: "rgba(255, 255, 255, 0.6)",
                            mt: 1.5,
                            fontSize: "0.8rem",
                            lineHeight: 1.5,
                            maxHeight: "3.6em",
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            display: "-webkit-box",
                            WebkitLineClamp: 2,
                            WebkitBoxOrient: "vertical",
                            px: 1.5,
                            py: 1,
                            borderRadius: "3px",
                            backgroundColor: "rgba(0, 0, 0, 0.2)",
                            borderLeft: "2px solid rgba(128, 90, 213, 0.3)",
                        }}
                    >
                        {summaryCaption}
                    </Typography>
                )}
            </Box>
        </Paper>
    );
};

export default SessionSummary;
