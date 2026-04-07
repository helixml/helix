import React, { useState, useEffect, useCallback, useRef } from "react";
import { useDropzone } from "react-dropzone";
import {
  Box,
  Typography,
  IconButton,
  CircularProgress,
  Alert,
} from "@mui/material";
import { Close as CloseIcon, Image as ImageIcon } from "@mui/icons-material";
import { useUploadFilestoreFiles, useDeleteFilestoreItem, useFilestoreConfig, getFilestoreViewerUrl } from "../../services/filestoreService";

const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB
const ACCEPTED_TYPES: Record<string, string[]> = {
  "image/png": [".png"],
  "image/jpeg": [".jpg", ".jpeg"],
  "image/gif": [".gif"],
  "image/webp": [".webp"],
};

export interface UploadedImage {
  id: string;
  filename: string;
  previewUrl: string;
  viewerUrl: string;
  filestorePath: string;
}

interface ImageAttachmentsProps {
  /** Called whenever the list of successfully uploaded images changes */
  onImagesChange: (images: UploadedImage[]) => void;
  /** Filestore path prefix for uploads (e.g. "task-attachments/{sessionId}") */
  uploadPath: string;
  /** Ref to a container element to listen for paste events on */
  pasteTargetRef?: React.RefObject<HTMLElement | null>;
}

interface PendingUpload {
  id: string;
  filename: string;
  previewUrl: string;
  uploading: boolean;
  error?: string;
}

const ImageAttachments: React.FC<ImageAttachmentsProps> = ({
  onImagesChange,
  uploadPath,
  pasteTargetRef,
}) => {
  const [images, setImages] = useState<UploadedImage[]>([]);
  const [pending, setPending] = useState<PendingUpload[]>([]);
  const [error, setError] = useState<string | null>(null);

  const uploadMutation = useUploadFilestoreFiles();
  const deleteMutation = useDeleteFilestoreItem();
  const { data: filestoreConfig } = useFilestoreConfig();

  // Notify parent when images change
  const imagesRef = useRef(images);
  useEffect(() => {
    imagesRef.current = images;
    onImagesChange(images);
  }, [images]);

  const processFiles = useCallback(
    async (files: File[]) => {
      setError(null);

      // Validate files
      const validFiles: File[] = [];
      for (const file of files) {
        if (file.size > MAX_FILE_SIZE) {
          setError(`"${file.name}" exceeds 10MB limit`);
          continue;
        }
        if (!Object.keys(ACCEPTED_TYPES).includes(file.type)) {
          setError(`"${file.name}" is not a supported image format (PNG, JPEG, GIF, WebP)`);
          continue;
        }
        validFiles.push(file);
      }

      if (validFiles.length === 0) return;

      // Create pending entries with previews
      const newPending: PendingUpload[] = validFiles.map((file) => ({
        id: crypto.randomUUID(),
        filename: file.name,
        previewUrl: URL.createObjectURL(file),
        uploading: true,
      }));
      setPending((prev) => [...prev, ...newPending]);

      // Upload each file
      for (let i = 0; i < validFiles.length; i++) {
        const file = validFiles[i];
        const pendingEntry = newPending[i];

        try {
          await uploadMutation.mutateAsync({
            path: uploadPath,
            files: [file],
            config: filestoreConfig || undefined,
          });

          // Build the filestore path for this file
          const userPrefix = filestoreConfig?.user_prefix || "";
          const fullPath = userPrefix
            ? `${userPrefix}/${uploadPath}/${file.name}`
            : `${uploadPath}/${file.name}`;
          const viewerUrl = getFilestoreViewerUrl(fullPath);

          const uploadedImage: UploadedImage = {
            id: pendingEntry.id,
            filename: file.name,
            previewUrl: pendingEntry.previewUrl,
            viewerUrl,
            filestorePath: fullPath,
          };

          setImages((prev) => [...prev, uploadedImage]);
          setPending((prev) => prev.filter((p) => p.id !== pendingEntry.id));
        } catch (err) {
          console.error(`Failed to upload ${file.name}:`, err);
          setPending((prev) =>
            prev.map((p) =>
              p.id === pendingEntry.id
                ? { ...p, uploading: false, error: "Upload failed" }
                : p,
            ),
          );
        }
      }
    },
    [uploadPath, filestoreConfig],
  );

  // Drag-and-drop
  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    accept: ACCEPTED_TYPES,
    maxSize: MAX_FILE_SIZE,
    onDrop: processFiles,
    noClick: images.length > 0 || pending.length > 0, // Only clickable when empty
  });

  // Clipboard paste handler
  useEffect(() => {
    const target = pasteTargetRef?.current;
    if (!target) return;

    const handlePaste = (e: Event) => {
      const clipboardEvent = e as ClipboardEvent;
      const items = clipboardEvent.clipboardData?.items;
      if (!items) return;

      const imageFiles: File[] = [];
      for (const item of Array.from(items)) {
        if (item.type.startsWith("image/")) {
          const file = item.getAsFile();
          if (file) {
            // Generate a meaningful filename for pasted images
            const ext = file.type.split("/")[1] || "png";
            const pastedFile = new File(
              [file],
              `screenshot-${Date.now()}.${ext}`,
              { type: file.type },
            );
            imageFiles.push(pastedFile);
          }
        }
      }

      if (imageFiles.length > 0) {
        e.preventDefault();
        processFiles(imageFiles);
      }
    };

    target.addEventListener("paste", handlePaste);
    return () => target.removeEventListener("paste", handlePaste);
  }, [pasteTargetRef?.current, processFiles]);

  const handleRemove = useCallback(
    async (image: UploadedImage) => {
      // Remove from UI immediately
      setImages((prev) => prev.filter((img) => img.id !== image.id));
      URL.revokeObjectURL(image.previewUrl);

      // Delete from filestore (best-effort, don't block UI)
      try {
        await deleteMutation.mutateAsync(image.filestorePath);
      } catch (err) {
        console.error("Failed to delete file from filestore:", err);
      }
    },
    [],
  );

  const handleRemovePending = useCallback((id: string) => {
    setPending((prev) => {
      const entry = prev.find((p) => p.id === id);
      if (entry) URL.revokeObjectURL(entry.previewUrl);
      return prev.filter((p) => p.id !== id);
    });
  }, []);

  const hasItems = images.length > 0 || pending.length > 0;

  return (
    <Box>
      {/* Drop zone */}
      <Box
        {...getRootProps()}
        sx={{
          border: "1px dashed",
          borderColor: isDragActive ? "primary.main" : "divider",
          borderRadius: 1,
          p: hasItems ? 1 : 2,
          cursor: "pointer",
          bgcolor: isDragActive ? "action.hover" : "transparent",
          transition: "all 0.2s",
          "&:hover": { borderColor: "text.secondary" },
        }}
      >
        <input {...getInputProps()} />

        {!hasItems && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              gap: 1,
            }}
          >
            <ImageIcon sx={{ fontSize: 20, color: "text.secondary" }} />
            <Typography variant="caption" color="text.secondary">
              Drop screenshots here, paste from clipboard, or click to browse
            </Typography>
          </Box>
        )}

        {/* Thumbnail grid */}
        {hasItems && (
          <Box
            sx={{
              display: "flex",
              flexWrap: "wrap",
              gap: 1,
            }}
          >
            {images.map((img) => (
              <Box
                key={img.id}
                sx={{
                  position: "relative",
                  width: 80,
                  height: 80,
                  borderRadius: 1,
                  overflow: "hidden",
                  border: 1,
                  borderColor: "divider",
                }}
              >
                <Box
                  component="img"
                  src={img.previewUrl}
                  alt={img.filename}
                  sx={{
                    width: "100%",
                    height: "100%",
                    objectFit: "cover",
                  }}
                />
                <IconButton
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleRemove(img);
                  }}
                  sx={{
                    position: "absolute",
                    top: 0,
                    right: 0,
                    bgcolor: "rgba(0,0,0,0.6)",
                    color: "white",
                    p: 0.25,
                    "&:hover": { bgcolor: "rgba(0,0,0,0.8)" },
                  }}
                >
                  <CloseIcon sx={{ fontSize: 14 }} />
                </IconButton>
              </Box>
            ))}

            {pending.map((p) => (
              <Box
                key={p.id}
                sx={{
                  position: "relative",
                  width: 80,
                  height: 80,
                  borderRadius: 1,
                  overflow: "hidden",
                  border: 1,
                  borderColor: p.error ? "error.main" : "divider",
                  opacity: p.uploading ? 0.6 : 1,
                }}
              >
                <Box
                  component="img"
                  src={p.previewUrl}
                  alt={p.filename}
                  sx={{
                    width: "100%",
                    height: "100%",
                    objectFit: "cover",
                  }}
                />
                {p.uploading && (
                  <Box
                    sx={{
                      position: "absolute",
                      inset: 0,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      bgcolor: "rgba(0,0,0,0.4)",
                    }}
                  >
                    <CircularProgress size={20} sx={{ color: "white" }} />
                  </Box>
                )}
                {p.error && (
                  <IconButton
                    size="small"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleRemovePending(p.id);
                    }}
                    sx={{
                      position: "absolute",
                      top: 0,
                      right: 0,
                      bgcolor: "rgba(0,0,0,0.6)",
                      color: "white",
                      p: 0.25,
                      "&:hover": { bgcolor: "rgba(0,0,0,0.8)" },
                    }}
                  >
                    <CloseIcon sx={{ fontSize: 14 }} />
                  </IconButton>
                )}
              </Box>
            ))}

            {/* Add more button */}
            <Box
              sx={{
                width: 80,
                height: 80,
                borderRadius: 1,
                border: "1px dashed",
                borderColor: "divider",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                cursor: "pointer",
                "&:hover": { borderColor: "text.secondary" },
              }}
            >
              <ImageIcon sx={{ fontSize: 20, color: "text.secondary" }} />
            </Box>
          </Box>
        )}
      </Box>

      {error && (
        <Alert severity="warning" sx={{ mt: 1 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
    </Box>
  );
};

export default ImageAttachments;
