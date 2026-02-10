import React, { FC, useState, useMemo } from "react";
import {
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Paper,
    Typography,
    Box,
    CircularProgress,
    Tooltip,
    TextField,
    Chip,
    Avatar,
    ToggleButton,
    ToggleButtonGroup,
} from "@mui/material";
import { TypesOpenAIModel, TypesProviderEndpoint, TypesDynamicModelInfo } from "../../api/api";
import { useListProviders } from "../../services/providersService";
import { useListModelInfos } from "../../services/modelInfoService";
import ModelPricingDialog from "./ModelPricingDialog";

import openaiLogo from '../../../assets/img/openai-logo.png';
import togetheraiLogo from '../../../assets/img/together-logo.png';
import vllmLogo from '../../../assets/img/vllm-logo.png';
import helixLogo from '../../../assets/img/logo.png';
import googleLogo from '../../../assets/img/providers/google.svg';
import anthropicLogo from '../../../assets/img/providers/anthropic.png';
import { Bot } from 'lucide-react';

interface ModelWithProvider extends TypesOpenAIModel {
    provider: TypesProviderEndpoint;
}

type PricingFilter = "all" | "with_pricing" | "without_pricing";

const getProviderIcon = (provider: TypesProviderEndpoint): React.ReactNode => {
    if (provider.base_url?.startsWith('https://api.openai.com/')) {
        return <Avatar src={openaiLogo} sx={{ width: 24, height: 24 }} variant="square" />;
    }
    if (provider.base_url?.startsWith('https://api.together.xyz/')) {
        return <Avatar src={togetheraiLogo} sx={{ width: 24, height: 24, bgcolor: '#fff' }} variant="square" />;
    }
    if (provider.base_url?.startsWith('https://generativelanguage.googleapis.com/')) {
        return <Avatar src={googleLogo} sx={{ width: 24, height: 24 }} variant="square" />;
    }
    if (provider.name === 'anthropic' || provider.base_url?.startsWith('https://api.anthropic.com/')) {
        return <Avatar src={anthropicLogo} sx={{ width: 24, height: 24 }} variant="square" />;
    }
    if (provider.available_models?.length && provider.available_models[0].owned_by === "vllm") {
        return <Avatar src={vllmLogo} sx={{ width: 24, height: 24, bgcolor: '#fff' }} variant="square" />;
    }
    if (provider.available_models?.length && provider.available_models[0].owned_by === "helix") {
        return <Avatar src={helixLogo} sx={{ width: 24, height: 24, bgcolor: '#fff' }} variant="square" />;
    }
    return (
        <Avatar sx={{ bgcolor: '#9E9E9E', width: 24, height: 24 }}>
            <Bot size={14} />
        </Avatar>
    );
};

const formatPrice = (perTokenPrice: string): string => {
    try {
        const price = parseFloat(perTokenPrice);
        if (isNaN(price)) return perTokenPrice;
        const perMillionPrice = price * 1000000;
        let formatted: string;
        if (perMillionPrice >= 1) {
            formatted = perMillionPrice.toFixed(2);
        } else if (perMillionPrice >= 0.01) {
            formatted = perMillionPrice.toFixed(4);
        } else {
            formatted = perMillionPrice.toFixed(6);
        }
        formatted = formatted.replace(/\.?0+$/, "");
        return `$${formatted}`;
    } catch {
        return perTokenPrice;
    }
};

const PricingTable: FC = () => {
    const [searchQuery, setSearchQuery] = useState("");
    const [pricingFilter, setPricingFilter] = useState<PricingFilter>("all");
    const [pricingDialogOpen, setPricingDialogOpen] = useState(false);
    const [selectedModel, setSelectedModel] = useState<ModelWithProvider | null>(null);

    const { data: providers, isLoading: isLoadingProviders } = useListProviders({
        loadModels: true,
        all: true,
    });

    const { data: modelInfos = [], refetch: refetchModelInfos } = useListModelInfos();

    const allModels: ModelWithProvider[] = useMemo(() => {
        return providers?.flatMap((provider: TypesProviderEndpoint) =>
            (provider.available_models || []).map((model: TypesOpenAIModel): ModelWithProvider => ({
                ...model,
                provider,
            }))
        ) ?? [];
    }, [providers]);

    const getModelPricingInfo = (model: ModelWithProvider): { prompt?: string; completion?: string; source: "provider" | "dynamic" | "none"; dynamicModelInfo?: TypesDynamicModelInfo } => {
        const providerPricing = model.model_info?.pricing;
        const hasProviderPricing = providerPricing && (providerPricing.prompt || providerPricing.completion);

        const dynamicInfo = modelInfos.find((info) => {
            if (info.provider === model.provider?.name && info.name === model.id) return true;
            const modelName = model.id?.replace(/^[^:]+:/, "");
            if (info.provider === model.provider?.name && info.name === modelName) return true;
            return false;
        });

        const hasDynamicPricing = dynamicInfo?.model_info?.pricing && (dynamicInfo.model_info.pricing.prompt || dynamicInfo.model_info.pricing.completion);

        if (hasDynamicPricing && dynamicInfo) {
            return {
                prompt: dynamicInfo.model_info?.pricing?.prompt,
                completion: dynamicInfo.model_info?.pricing?.completion,
                source: "dynamic",
                dynamicModelInfo: dynamicInfo,
            };
        }

        if (hasProviderPricing) {
            return {
                prompt: providerPricing?.prompt,
                completion: providerPricing?.completion,
                source: "provider",
            };
        }

        return { source: "none" };
    };

    const filteredModels = useMemo(() => {
        let models = allModels;

        if (searchQuery) {
            const query = searchQuery.toLowerCase();
            models = models.filter((model) =>
                model.id?.toLowerCase().includes(query) ||
                model.provider?.name?.toLowerCase().includes(query) ||
                model.description?.toLowerCase().includes(query)
            );
        }

        if (pricingFilter !== "all") {
            models = models.filter((model) => {
                const pricing = getModelPricingInfo(model);
                if (pricingFilter === "with_pricing") {
                    return pricing.source !== "none";
                }
                return pricing.source === "none";
            });
        }

        return models;
    }, [allModels, searchQuery, pricingFilter, modelInfos]);

    const handleSetPrice = (model: ModelWithProvider) => {
        setSelectedModel(model);
        setPricingDialogOpen(true);
    };

    const getModelInfoForPricing = (model: ModelWithProvider | null): TypesDynamicModelInfo | undefined => {
        if (!model) return undefined;

        const pricingInfo = getModelPricingInfo(model);

        if (pricingInfo.dynamicModelInfo) {
            return pricingInfo.dynamicModelInfo;
        }

        return {
            provider: model.provider?.name || "",
            name: model.id || "",
            model_info: {
                name: model.id || "",
                pricing: pricingInfo.source === "provider" ? {
                    prompt: pricingInfo.prompt,
                    completion: pricingInfo.completion,
                } : {},
            },
        };
    };

    const handlePricingUpdateSuccess = () => {
        refetchModelInfos();
        setTimeout(() => {
            refetchModelInfos();
        }, 500);
    };

    if (isLoadingProviders) {
        return (
            <Paper sx={{ p: 2, display: "flex", justifyContent: "center", alignItems: "center" }}>
                <CircularProgress />
            </Paper>
        );
    }

    return (
        <Paper sx={{ width: "100%", overflow: "hidden" }}>
            <Box sx={{ p: 2, display: "flex", justifyContent: "space-between", alignItems: "center", gap: 2 }}>
                <TextField
                    label="Search by model ID, provider, or description"
                    size="small"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    sx={{ width: "40%" }}
                />
                <ToggleButtonGroup
                    value={pricingFilter}
                    exclusive
                    onChange={(_, value) => value && setPricingFilter(value)}
                    size="small"
                >
                    <ToggleButton value="all">All</ToggleButton>
                    <ToggleButton value="with_pricing">Has Pricing</ToggleButton>
                    <ToggleButton value="without_pricing">No Pricing</ToggleButton>
                </ToggleButtonGroup>
            </Box>
            <ModelPricingDialog
                open={pricingDialogOpen}
                model={getModelInfoForPricing(selectedModel)}
                onClose={() => {
                    setPricingDialogOpen(false);
                    setSelectedModel(null);
                }}
                refreshData={handlePricingUpdateSuccess}
            />
            <TableContainer>
                <Table stickyHeader aria-label="pricing table" size="small">
                    <TableHead>
                        <TableRow>
                            <TableCell>Provider</TableCell>
                            <TableCell>Model</TableCell>
                            <TableCell>Type</TableCell>
                            <TableCell align="center">Input Price (per 1M tokens)</TableCell>
                            <TableCell align="center">Output Price (per 1M tokens)</TableCell>
                            <TableCell align="center">Source</TableCell>
                            <TableCell align="center">Actions</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {filteredModels.map((model) => {
                            const pricing = getModelPricingInfo(model);
                            return (
                                <TableRow key={`${model.provider?.name}-${model.id}`} hover>
                                    <TableCell>
                                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                                            {getProviderIcon(model.provider)}
                                            <Typography variant="body2">
                                                {model.provider?.name || "unknown"}
                                            </Typography>
                                        </Box>
                                    </TableCell>
                                    <TableCell>
                                        <Tooltip title={model.description || model.id || ""}>
                                            <Typography
                                                variant="body2"
                                                sx={{
                                                    maxWidth: 300,
                                                    overflow: "hidden",
                                                    textOverflow: "ellipsis",
                                                    whiteSpace: "nowrap",
                                                }}
                                            >
                                                {model.id || "N/A"}
                                            </Typography>
                                        </Tooltip>
                                    </TableCell>
                                    <TableCell>
                                        <Typography variant="body2" color="text.secondary">
                                            {model.type || "-"}
                                        </Typography>
                                    </TableCell>
                                    <TableCell align="center">
                                        <Typography variant="body2" sx={{ fontWeight: pricing.prompt ? "medium" : "normal" }}>
                                            {pricing.prompt ? formatPrice(pricing.prompt) : "-"}
                                        </Typography>
                                    </TableCell>
                                    <TableCell align="center">
                                        <Typography variant="body2" sx={{ fontWeight: pricing.completion ? "medium" : "normal" }}>
                                            {pricing.completion ? formatPrice(pricing.completion) : "-"}
                                        </Typography>
                                    </TableCell>
                                    <TableCell align="center">
                                        {pricing.source === "dynamic" && (
                                            <Chip label="Custom" size="small" color="success" variant="outlined" />
                                        )}
                                        {pricing.source === "provider" && (
                                            <Chip label="Provider" size="small" color="default" variant="outlined" />
                                        )}
                                        {pricing.source === "none" && (
                                            <Chip label="Not Set" size="small" color="error" variant="outlined" />
                                        )}
                                    </TableCell>
                                    <TableCell align="center">
                                        <Chip
                                            label={pricing.source === "none" ? "Set Price" : "Edit Price"}
                                            size="small"
                                            color={pricing.source === "none" ? "primary" : "default"}
                                            onClick={() => handleSetPrice(model)}
                                            clickable
                                        />
                                    </TableCell>
                                </TableRow>
                            );
                        })}
                    </TableBody>
                </Table>
            </TableContainer>
            {filteredModels.length === 0 && (
                <Box sx={{ p: 4, textAlign: "center" }}>
                    <Typography color="text.secondary">
                        {searchQuery || pricingFilter !== "all"
                            ? "No models match the current filters."
                            : "No models available."}
                    </Typography>
                </Box>
            )}
        </Paper>
    );
};

export default PricingTable;
