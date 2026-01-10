import React, { useState, useCallback, useEffect } from "react";
import {
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Button,
    TextField,
    FormControl,
    InputLabel,
    Select,
    MenuItem,
    Alert,
    Stack,
    SelectChangeEvent,
    CircularProgress,
    Switch,
    FormControlLabel,
    FormHelperText,
    Link,
    Box,
    Typography,
    LinearProgress,
    Slider,
} from "@mui/material";
import DownloadIcon from "@mui/icons-material/Download";
// Use actual types from the API definition
import {
    TypesModel,
    TypesModelType,
    GithubComHelixmlHelixApiPkgTypesRuntime as TypesRuntime,
    TypesRunnerModelStatus,
} from "../../api/api";
import {
    useCreateHelixModel,
    useUpdateHelixModel,
} from "../../services/helixModelsService";
import { useGetDashboardData } from "../../services/dashboardService";
import MemoryEstimationWidget from "./MemoryEstimationWidget";

interface EditHelixModelDialogProps {
    open: boolean;
    model: TypesModel | null; // null for creating a new model
    onClose: () => void;
    refreshData: () => void; // To refresh the list after save
}

const EditHelixModelDialog: React.FC<EditHelixModelDialogProps> = ({
    open,
    model,
    onClose,
    refreshData,
}) => {
    const isEditing = !!model;
    // Placeholder hooks - replace with your actual implementation
    const { mutateAsync: createModel, isPending: isCreating } =
        useCreateHelixModel();
    const { mutateAsync: updateModel, isPending: isUpdating } =
        useUpdateHelixModel();
    const { data: dashboardData } = useGetDashboardData();
    const loading = isCreating || isUpdating;

    const [error, setError] = useState<string>("");
    const [isModelRegistered, setIsModelRegistered] = useState(false);
    // Store memory in GB in the form state
    const [formData, setFormData] = useState({
        id: "",
        name: "",
        description: "",
        type: TypesModelType.ModelTypeChat as TypesModelType,
        runtime: TypesRuntime.RuntimeOllama as TypesRuntime,
        memory: 0, // Store memory in GB
        context_length: 0, // Initialize context length
        concurrency: 0, // Number of concurrent requests (0 = use runtime default)
        enabled: true,
        hide: false,
        auto_pull: true, // Enable auto_pull by default
        prewarm: false, // Add prewarm state
        runtime_args: "", // Store VLLM args as JSON string for editing
    });

    // Initialize form data when the dialog opens or the model changes
    useEffect(() => {
        if (open) {
            setIsModelRegistered(false); // Reset registration state when dialog opens
            if (isEditing && model) {
                setFormData({
                    id: model.id || "",
                    name: model.name || "",
                    description: model.description || "",
                    type: model.type || TypesModelType.ModelTypeChat,
                    runtime: model.runtime || TypesRuntime.RuntimeOllama,
                    memory: model.memory
                        ? model.memory / (1024 * 1024 * 1024)
                        : 0, // Convert bytes to GB
                    context_length: model.context_length || 0,
                    concurrency: model.concurrency || 0, // Initialize concurrency
                    enabled:
                        model.enabled !== undefined ? model.enabled : false, // Default to false if undefined
                    hide: model.hide || false,
                    auto_pull: model.auto_pull || false, // Initialize auto_pull
                    prewarm: model.prewarm || false, // Initialize prewarm
                    runtime_args: model.runtime_args
                        ? JSON.stringify(model.runtime_args, null, 2)
                        : "", // Convert to formatted JSON string
                });
            } else {
                // Reset for creating a new model
                setFormData({
                    id: "",
                    name: "",
                    description: "",
                    type: TypesModelType.ModelTypeChat,
                    runtime: TypesRuntime.RuntimeOllama,
                    memory: 0, // Default GB
                    context_length: 32000, // Default to 32K tokens
                    concurrency: 0, // Number of concurrent requests (0 = use runtime default)
                    enabled: true,
                    hide: false,
                    auto_pull: true, // Enable auto_pull by default
                    prewarm: false, // Default prewarm to false
                    runtime_args: "", // Default empty runtime args
                });
            }
            setError(""); // Clear errors when dialog opens/changes mode
        }
    }, [open, model, isEditing]);

    // Helper function to generate a user-friendly name from a model ID
    const generateModelNameFromId = (modelId: string): string => {
        if (!modelId.trim()) return "";

        // Handle different patterns
        let name = modelId;

        // Handle provider/model format (e.g., "nvidia/Nemotron-CC-v2")
        if (name.includes("/")) {
            const parts = name.split("/");
            name = parts.join(" ");
        }

        // Handle colon format (e.g., "gemma3:12b", "llama3:70b-instruct-q4_0")
        if (name.includes(":")) {
            name = name.replace(":", " ");
        }

        // Replace hyphens and underscores with spaces
        name = name.replace(/[-_]/g, " ");

        // Capitalize each word first
        name = name
            .split(" ")
            .map(
                (word) =>
                    word.charAt(0).toUpperCase() + word.slice(1).toLowerCase(),
            )
            .join(" ");

        // Convert size indicators after capitalization (e.g., "12b" -> "12B", "70b" -> "70B")
        name = name.replace(/\b(\d+(?:\.\d+)?)b\b/gi, "$1B");

        return name;
    };

    const handleTextFieldChange = (
        e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
    ) => {
        const { name, value } = e.target;
        // Handle number inputs specifically
        if (name === "memory") {
            // Parse as float for GB
            const floatValue = value === "" ? 0 : parseFloat(value);
            if (!isNaN(floatValue) && floatValue >= 0) {
                setFormData((prev) => ({ ...prev, [name]: floatValue }));
            } else if (value === "") {
                // Allow clearing the field, treating it as 0 for state
                setFormData((prev) => ({ ...prev, [name]: 0 }));
            }
        } else if (name === "context_length") {
            const numValue = value === "" ? 0 : parseInt(value, 10);
            if (!isNaN(numValue) && numValue >= 0) {
                setFormData((prev) => ({ ...prev, [name]: numValue }));
            } else if (value === "") {
                // Allow clearing the field, treating it as 0 for state
                setFormData((prev) => ({ ...prev, [name]: 0 }));
            }
        } else {
            // Handle model ID changes - auto-fill name if empty
            if (name === "id" && !isEditing) {
                setFormData((prev) => {
                    const newFormData = { ...prev, [name]: value };
                    // Only auto-fill name if it's currently empty
                    if (!prev.name.trim() && value.trim()) {
                        newFormData.name = generateModelNameFromId(value);
                    }
                    return newFormData;
                });
            } else {
                setFormData((prev) => ({ ...prev, [name]: value }));
            }
        }
        setError("");
    };

    const handleSelectChange = (e: SelectChangeEvent<string>) => {
        const { name, value } = e.target;
        setFormData((prev) => ({
            ...prev,
            [name]: value as TypesModelType | TypesRuntime, // Cast based on the field name
        }));
        setError("");
    };

    const handleSwitchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const { name, checked } = e.target;
        setFormData((prev) => ({ ...prev, [name]: checked }));
        setError("");
    };

    const validateForm = useCallback(() => {
        // Only validate ID when creating a new model
        if (!isEditing && !formData.id.trim()) {
            setError("Model ID is required");
            return false;
        }
        if (!formData.name.trim()) {
            setError("Model Name is required");
            return false;
        }
        if (!formData.type) {
            setError("Model Type is required");
            return false;
        }
        if (!formData.runtime) {
            setError("Runtime is required");
            return false;
        }
        // Only validate memory for non-Ollama models since Ollama auto-detects memory
        if (
            formData.runtime !== TypesRuntime.RuntimeOllama &&
            formData.memory <= 0
        ) {
            setError(
                "Memory (in GB) is required and must be a positive number.",
            );
            return false;
        }
        // Context length is optional, but should be non-negative if provided
        if (formData.context_length < 0) {
            setError("Context Length must be a non-negative number.");
            return false;
        }

        return true;
    }, [formData, isEditing]);

    const handleSubmit = async () => {
        if (!validateForm()) return;

        setError(""); // Clear previous errors

        // Parse runtime_args if provided
        let runtimeArgs: any = undefined;
        if (formData.runtime_args.trim()) {
            try {
                runtimeArgs = JSON.parse(formData.runtime_args);
            } catch (error) {
                setError("Invalid JSON in Runtime Args field");
                return;
            }
        }

        // Construct the base payload
        const payloadBase: Partial<TypesModel> = {
            // ID is handled differently for create vs update below
            name: formData.name.trim(),
            description: formData.description.trim() || undefined,
            type: formData.type,
            runtime: formData.runtime,
            memory:
                formData.runtime === TypesRuntime.RuntimeOllama
                    ? 0 // Will be auto-detected after download
                    : Math.round(formData.memory * 1024 * 1024 * 1024), // Convert GB to Bytes
            context_length:
                formData.context_length > 0
                    ? formData.context_length
                    : undefined,
            concurrency:
                formData.concurrency > 0 ? formData.concurrency : undefined, // 0 means use runtime default
            enabled: formData.enabled,
            auto_pull: formData.auto_pull,
            prewarm: formData.prewarm,
            hide: formData.hide,
            runtime_args: runtimeArgs,
        };

        try {
            if (isEditing && model) {
                // For update, use the existing model's ID (which is not editable)
                await updateModel({
                    id: model.id || "",
                    helixModel: { ...payloadBase },
                });
            } else if (isModelRegistered) {
                // Model was already registered via Pull button, update it
                await updateModel({
                    id: formData.id.trim(),
                    helixModel: { ...payloadBase },
                });
            } else {
                // For create, include the user-provided ID from the form
                await createModel({ ...payloadBase, id: formData.id.trim() });
            }
            refreshData(); // Refresh the data in the parent component
            onClose(); // Close the dialog on success
        } catch (err) {
            console.error("Failed to save model:", err);
            setError(
                err instanceof Error ? err.message : "Failed to save model",
            );
        }
    };

    const handleClose = () => {
        // No need to reset form here if useEffect handles it based on `open`
        onClose();
    };

    // Get download progress for the current model across all runners
    const getModelDownloadStatus = useCallback(() => {
        if (!formData.id || !dashboardData?.runners) return null;

        const modelStatuses: TypesRunnerModelStatus[] = [];
        dashboardData.runners.forEach((runner) => {
            if (runner.models) {
                const modelStatus = runner.models.find(
                    (m) => m.model_id === formData.id,
                );
                if (modelStatus) {
                    modelStatuses.push(modelStatus);
                }
            }
        });

        if (modelStatuses.length === 0) return null;

        const downloadingRunners = modelStatuses.filter(
            (s) => s.download_in_progress,
        );
        const completedRunners = modelStatuses.filter(
            (s) => !s.download_in_progress && !s.error,
        );
        const errorRunners = modelStatuses.filter((s) => s.error);

        return {
            total: modelStatuses.length,
            downloading: downloadingRunners.length,
            completed: completedRunners.length,
            errors: errorRunners.length,
            hasCompleted: completedRunners.length > 0,
            allCompleted: completedRunners.length === modelStatuses.length,
            statuses: modelStatuses,
        };
    }, [formData.id, dashboardData?.runners]);

    const downloadStatus = getModelDownloadStatus();

    // Common context length presets in K (thousands)
    const CONTEXT_PRESETS = [4, 8, 16, 32, 64, 128, 256, 512];

    // Convert context length to/from logarithmic scale for better slider distribution
    const contextToLogScale = (contextK: number) =>
        Math.log10(Math.max(contextK, 1));
    const logScaleToContext = (logValue: number) => Math.pow(10, logValue);

    const handleContextSliderChange = useCallback(
        (_: Event, value: number | number[]) => {
            const logValue = Array.isArray(value) ? value[0] : value;
            const contextK = logScaleToContext(logValue);

            // Snap to preset if close enough
            const snapThreshold = contextK * 0.1;
            const closestPreset = CONTEXT_PRESETS.reduce((closest, preset) => {
                const distance = Math.abs(contextK - preset);
                const closestDistance = Math.abs(contextK - closest);
                return distance < closestDistance ? preset : closest;
            });

            const shouldSnap =
                Math.abs(contextK - closestPreset) <= snapThreshold;
            const finalContextK = shouldSnap
                ? closestPreset
                : Math.round(contextK);
            const contextLength = finalContextK * 1000;

            setFormData((prev) => ({ ...prev, context_length: contextLength }));
        },
        [],
    );

    const handlePullModel = async () => {
        if (!validateForm()) return;

        setError(""); // Clear previous errors

        // Parse runtime_args if provided
        let runtimeArgs: any = undefined;
        if (formData.runtime_args.trim()) {
            try {
                runtimeArgs = JSON.parse(formData.runtime_args);
            } catch (error) {
                setError("Invalid JSON in Runtime Args field");
                return;
            }
        }

        // Register the actual model with auto_pull enabled
        const modelPayload: Partial<TypesModel> = {
            id: formData.id.trim(),
            name: formData.name.trim(),
            description: formData.description.trim() || undefined,
            type: formData.type,
            runtime: formData.runtime,
            memory: 0, // Will be auto-detected after download
            context_length:
                formData.context_length > 0
                    ? formData.context_length
                    : undefined,
            enabled: formData.enabled,
            auto_pull: true, // Always enable auto_pull for download
            prewarm: formData.prewarm,
            hide: formData.hide,
            runtime_args: runtimeArgs,
        };

        try {
            await createModel(modelPayload);
            // Model is now registered and will start downloading
            setIsModelRegistered(true);
            // Keep dialog open to show progress
        } catch (err) {
            console.error("Failed to register model:", err);
            setError(
                err instanceof Error ? err.message : "Failed to register model",
            );
        }
    };

    return (
        <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
            <DialogTitle>
                {isEditing
                    ? `Edit Helix Model: ${model?.name}`
                    : "Register New Helix Model"}
            </DialogTitle>
            <DialogContent>
                <Stack spacing={3} sx={{ mt: 2 }}>
                    {error && <Alert severity="error">{error}</Alert>}

                    {isEditing && model && (
                        <TextField
                            label="Model ID"
                            value={model.id}
                            fullWidth
                            disabled // ID is not editable when editing
                            sx={{ mb: 2 }} // Add margin if needed
                        />
                    )}

                    {!isEditing && (
                        <TextField
                            name="id"
                            label="Model ID"
                            value={formData.id}
                            onChange={handleTextFieldChange}
                            fullWidth
                            required
                            autoComplete="off"
                            placeholder="e.g., llama3:70b-instruct-q4_0 or custom-model-name"
                            helperText={
                                <>
                                    Unique identifier for the model (may be
                                    provider-specific). For example{" "}
                                    <strong>qwen3:8b</strong> for Ollama or{" "}
                                    <strong>nvidia/Nemotron-CC-v2</strong> for
                                    vLLM which downloads from HuggingFace. Find
                                    Ollama models at{" "}
                                    <Link
                                        href="https://ollama.com/search"
                                        target="_blank"
                                        rel="noopener noreferrer"
                                    >
                                        https://ollama.com/search
                                    </Link>
                                </>
                            }
                            disabled={loading}
                        />
                    )}

                    <TextField
                        name="name"
                        label="Model Name"
                        value={formData.name}
                        onChange={handleTextFieldChange}
                        fullWidth
                        required
                        autoComplete="off"
                        placeholder="e.g., Llama 3 70B Instruct Q4"
                        helperText="User-friendly name shown in model selection lists."
                        disabled={loading}
                    />

                    <TextField
                        name="description"
                        label="Description"
                        value={formData.description}
                        onChange={handleTextFieldChange}
                        fullWidth
                        multiline
                        rows={2}
                        autoComplete="off"
                        placeholder="Optional: A brief description of the model or its purpose."
                        disabled={loading}
                    />

                    <FormControl fullWidth required disabled={loading}>
                        <InputLabel id="model-type-label">
                            Model Type
                        </InputLabel>
                        <Select
                            labelId="model-type-label"
                            name="type" // Changed from model_type
                            value={formData.type}
                            onChange={handleSelectChange}
                            label="Model Type"
                        >
                            <MenuItem value={TypesModelType.ModelTypeChat}>
                                Chat
                            </MenuItem>
                            <MenuItem value={TypesModelType.ModelTypeImage}>
                                Image
                            </MenuItem>
                            <MenuItem value={TypesModelType.ModelTypeEmbed}>
                                Embed
                            </MenuItem>
                        </Select>
                    </FormControl>

                    <FormControl fullWidth required disabled={loading}>
                        <InputLabel id="runtime-label">Runtime</InputLabel>
                        <Select
                            labelId="runtime-label"
                            name="runtime"
                            value={formData.runtime}
                            onChange={handleSelectChange}
                            label="Runtime"
                        >
                            {/* Use enum values */}
                            <MenuItem value={TypesRuntime.RuntimeOllama}>
                                Ollama
                            </MenuItem>
                            <MenuItem value={TypesRuntime.RuntimeVLLM}>
                                vLLM
                            </MenuItem>
                            <MenuItem
                                value={TypesRuntime.RuntimeDiffusers}
                                disabled
                            >
                                Diffusers (Image) (coming soon)
                            </MenuItem>
                            {/* Add more runtime options if they become available in the enum */}
                        </Select>
                    </FormControl>

                    {/* Pull button for Ollama models - right under runtime dropdown */}
                    {!isEditing &&
                        formData.runtime === TypesRuntime.RuntimeOllama && (
                            <Box>
                                <Button
                                    variant="contained"
                                    color={
                                        isModelRegistered
                                            ? "success"
                                            : "primary"
                                    }
                                    size="large"
                                    startIcon={<DownloadIcon />}
                                    onClick={handlePullModel}
                                    disabled={
                                        loading ||
                                        !formData.id.trim() ||
                                        isModelRegistered
                                    }
                                    fullWidth
                                    sx={{
                                        py: 1.5,
                                        fontSize: "1.1rem",
                                        fontWeight: "bold",
                                        boxShadow: 2,
                                        "&:hover": {
                                            boxShadow: 4,
                                        },
                                    }}
                                >
                                    {isModelRegistered
                                        ? "✓ Model Registered"
                                        : "Register & Pull Model"}
                                </Button>
                                <FormHelperText>
                                    {isModelRegistered
                                        ? "Model registered successfully! Download progress will appear below."
                                        : !formData.id.trim()
                                          ? "Enter a Model ID above to register and pull the model"
                                          : "Register the model and start downloading. Progress will be shown below."}
                                </FormHelperText>
                            </Box>
                        )}

                    {/* Download progress for Ollama models */}
                    {!isEditing &&
                        formData.runtime === TypesRuntime.RuntimeOllama &&
                        formData.id &&
                        isModelRegistered &&
                        downloadStatus && (
                            <Box>
                                <Typography variant="subtitle2" gutterBottom>
                                    Download Status Across Runners
                                </Typography>
                                {downloadStatus.downloading > 0 && (
                                    <Box sx={{ mb: 2 }}>
                                        <Box
                                            sx={{
                                                display: "flex",
                                                alignItems: "center",
                                                mb: 1,
                                            }}
                                        >
                                            <Typography
                                                variant="body2"
                                                sx={{ mr: 2 }}
                                            >
                                                Downloading on{" "}
                                                {downloadStatus.downloading}{" "}
                                                runner(s)
                                            </Typography>
                                            <CircularProgress size={16} />
                                        </Box>
                                        {downloadStatus.statuses
                                            .filter(
                                                (s) => s.download_in_progress,
                                            )
                                            .map((status, index) => (
                                                <Box key={index} sx={{ mb: 1 }}>
                                                    <Typography
                                                        variant="caption"
                                                        color="text.secondary"
                                                    >
                                                        Runner {index + 1}:{" "}
                                                        {status.download_percent ||
                                                            0}
                                                        %
                                                    </Typography>
                                                    <LinearProgress
                                                        variant="determinate"
                                                        value={
                                                            status.download_percent ||
                                                            0
                                                        }
                                                        sx={{ mt: 0.5 }}
                                                    />
                                                </Box>
                                            ))}
                                    </Box>
                                )}
                                {downloadStatus.completed > 0 && (
                                    <Typography
                                        variant="body2"
                                        color="success.main"
                                        sx={{ mb: 1 }}
                                    >
                                        ✓ Downloaded on{" "}
                                        {downloadStatus.completed} runner(s)
                                    </Typography>
                                )}
                                {downloadStatus.errors > 0 && (
                                    <Typography
                                        variant="body2"
                                        color="error.main"
                                        sx={{ mb: 1 }}
                                    >
                                        ✗ Failed on {downloadStatus.errors}{" "}
                                        runner(s)
                                    </Typography>
                                )}
                            </Box>
                        )}

                    {/* Memory field - only for non-Ollama models */}
                    {formData.runtime !== TypesRuntime.RuntimeOllama && (
                        <TextField
                            name="memory"
                            label="Memory Required (GB)"
                            type="number" // Allows decimals
                            value={formData.memory === 0 ? "" : formData.memory} // Show empty if 0
                            onChange={handleTextFieldChange}
                            fullWidth
                            required
                            autoComplete="off"
                            placeholder="e.g., 16 for 16GB"
                            helperText="Estimated memory needed to run this model (in GB)."
                            disabled={loading}
                            InputProps={{
                                inputProps: {
                                    min: 0, // Allow 0 input, validation ensures > 0 on submit
                                    step: "any", // Allow floating point numbers
                                },
                            }}
                        />
                    )}

                    {/* Context Length - show both slider and text field for Ollama, text field only for others */}
                    {formData.type === TypesModelType.ModelTypeChat && (
                        <>
                            {formData.runtime === TypesRuntime.RuntimeOllama ? (
                                <Box>
                                    <Typography
                                        variant="subtitle2"
                                        gutterBottom
                                        sx={{ mb: 1 }}
                                    >
                                        Context Length:{" "}
                                        <Box
                                            component="span"
                                            sx={{
                                                fontWeight: "bold",
                                                color: "primary.main",
                                            }}
                                        >
                                            {(
                                                formData.context_length / 1000
                                            ).toLocaleString()}
                                            K
                                        </Box>{" "}
                                        tokens
                                    </Typography>
                                    <Box sx={{ px: 1, maxWidth: 600 }}>
                                        <Slider
                                            value={contextToLogScale(
                                                Math.max(
                                                    formData.context_length /
                                                        1000,
                                                    4,
                                                ),
                                            )}
                                            onChange={handleContextSliderChange}
                                            min={contextToLogScale(4)}
                                            max={contextToLogScale(512)}
                                            step={0.01}
                                            disabled={loading}
                                            marks={CONTEXT_PRESETS.map(
                                                (preset) => ({
                                                    value: contextToLogScale(
                                                        preset,
                                                    ),
                                                    label: `${preset}K`,
                                                }),
                                            )}
                                            valueLabelDisplay="off"
                                        />
                                    </Box>
                                    <TextField
                                        name="context_length"
                                        label="Exact Context Length"
                                        type="number"
                                        value={
                                            formData.context_length === 0
                                                ? ""
                                                : formData.context_length
                                        }
                                        onChange={handleTextFieldChange}
                                        fullWidth
                                        autoComplete="off"
                                        placeholder="e.g., 128000"
                                        required
                                        helperText={
                                            downloadStatus?.hasCompleted
                                                ? "Maximum context window size (in tokens). Full memory estimation available."
                                                : "Maximum context window size (in tokens). Use slider for quick selection."
                                        }
                                        disabled={loading}
                                        sx={{ mt: 2 }}
                                        InputProps={{
                                            inputProps: {
                                                min: 0,
                                            },
                                        }}
                                    />
                                </Box>
                            ) : (
                                <TextField
                                    name="context_length"
                                    label="Context Length"
                                    type="number"
                                    value={
                                        formData.context_length === 0
                                            ? ""
                                            : formData.context_length
                                    }
                                    onChange={handleTextFieldChange}
                                    fullWidth
                                    autoComplete="off"
                                    placeholder="e.g., 128000"
                                    required
                                    helperText="Maximum context window size (in tokens)"
                                    disabled={loading}
                                    InputProps={{
                                        inputProps: {
                                            min: 0,
                                        },
                                    }}
                                />
                            )}
                        </>
                    )}

                    {/* Concurrency field - for Ollama models */}
                    {formData.runtime === TypesRuntime.RuntimeOllama && (
                        <TextField
                            name="concurrency"
                            label="Concurrent Requests"
                            type="number"
                            value={
                                formData.concurrency === 0
                                    ? ""
                                    : formData.concurrency
                            }
                            onChange={handleTextFieldChange}
                            fullWidth
                            autoComplete="off"
                            placeholder="0 (use runtime default)"
                            helperText={
                                formData.concurrency === 0 ||
                                !formData.concurrency
                                    ? "Number of concurrent requests this model can handle (0 = use runtime default of 2)"
                                    : `This model will handle up to ${formData.concurrency} concurrent requests. Higher values use more memory.`
                            }
                            disabled={loading}
                            InputProps={{
                                inputProps: {
                                    min: 0,
                                    max: 16, // Reasonable upper limit
                                },
                            }}
                        />
                    )}

                    {/* Memory Estimation Widget for Ollama models - only show after download completes */}
                    {formData.runtime === TypesRuntime.RuntimeOllama &&
                        formData.id &&
                        formData.type === TypesModelType.ModelTypeChat &&
                        downloadStatus?.hasCompleted && (
                            <MemoryEstimationWidget
                                modelId={formData.id}
                                currentContextLength={formData.context_length}
                                onContextLengthChange={(value) =>
                                    setFormData((prev) => ({
                                        ...prev,
                                        context_length: value,
                                    }))
                                }
                                currentConcurrency={formData.concurrency}
                                onConcurrencyChange={(value) =>
                                    setFormData((prev) => ({
                                        ...prev,
                                        concurrency: value,
                                    }))
                                }
                                disabled={loading}
                            />
                        )}

                    {/* Conditionally render Runtime Args field for VLLM runtime */}
                    {formData.runtime === TypesRuntime.RuntimeVLLM && (
                        <TextField
                            name="runtime_args"
                            label="Runtime Arguments (JSON)"
                            value={formData.runtime_args}
                            onChange={handleTextFieldChange}
                            fullWidth
                            multiline
                            rows={6}
                            autoComplete="off"
                            placeholder='["--trust-remote-code", "--max-model-len", "32768"]'
                            helperText="VLLM runtime arguments in JSON format. Note that --gpu-memory-utilization is automatically calculated and added based on model memory requirements and available GPU memory, so you do not need to and should not specify this parameter."
                            disabled={loading}
                        />
                    )}

                    <Stack
                        direction="row"
                        spacing={2}
                        justifyContent="start"
                        alignItems="center"
                    >
                        <FormControlLabel
                            control={
                                <Switch
                                    checked={formData.enabled}
                                    onChange={handleSwitchChange}
                                    name="enabled"
                                    disabled={loading}
                                />
                            }
                            label="Enabled"
                        />
                        <FormHelperText>
                            Allow users to select and use this model.
                        </FormHelperText>
                    </Stack>

                    <Stack
                        direction="row"
                        spacing={2}
                        justifyContent="start"
                        alignItems="center"
                    >
                        <FormControlLabel
                            control={
                                <Switch
                                    checked={formData.auto_pull}
                                    onChange={handleSwitchChange}
                                    name="auto_pull"
                                    disabled={loading}
                                />
                            }
                            label="Auto Pull"
                        />
                        <FormHelperText>
                            Automatically pull this model when it's not already
                            downloaded.
                        </FormHelperText>
                    </Stack>

                    <Stack
                        direction="row"
                        spacing={2}
                        justifyContent="start"
                        alignItems="center"
                    >
                        <FormControlLabel
                            control={
                                <Switch
                                    checked={formData.prewarm}
                                    onChange={handleSwitchChange}
                                    name="prewarm"
                                    disabled={loading}
                                />
                            }
                            label="Prewarm"
                        />
                        <FormHelperText>
                            Fill free GPU memory on runners with this model when
                            available.
                        </FormHelperText>
                    </Stack>

                    <Stack
                        direction="row"
                        spacing={2}
                        justifyContent="start"
                        alignItems="center"
                    >
                        <FormControlLabel
                            control={
                                <Switch
                                    checked={formData.hide}
                                    onChange={handleSwitchChange}
                                    name="hide"
                                    disabled={loading}
                                />
                            }
                            label="Hide"
                        />
                        <FormHelperText>
                            Hide this model from the default selection lists
                            (still usable if selected directly).
                        </FormHelperText>
                    </Stack>
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button onClick={handleClose} disabled={loading}>
                    Cancel
                </Button>
                <Button
                    onClick={handleSubmit}
                    variant="contained"
                    color="primary"
                    disabled={loading}
                    startIcon={loading ? <CircularProgress size={20} /> : null}
                >
                    {loading
                        ? isEditing
                            ? "Saving..."
                            : isModelRegistered
                              ? "Updating..."
                              : "Creating..."
                        : isEditing
                          ? "Save Changes"
                          : isModelRegistered
                            ? "Update Model"
                            : "Register Model"}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default EditHelixModelDialog;
