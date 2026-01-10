import React, { useState, useCallback, useRef, useEffect, FC } from 'react';
import { Box, LinearProgress, Typography, Fade, Snackbar, Alert } from '@mui/material';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CheckIcon from '@mui/icons-material/Check';
import ImageIcon from '@mui/icons-material/Image';

interface UploadState {
  progress: number; // 0-100 for progress, -1 for error, 101 for complete
  name: string;
}

interface SandboxDropZoneProps {
  sessionId: string;
  children: React.ReactNode;
  disabled?: boolean;
  // Called with the full path when a file is uploaded (for pasted images, etc.)
  onFileUploaded?: (filePath: string) => void;
}

const SandboxDropZone: FC<SandboxDropZoneProps> = ({
  sessionId,
  children,
  disabled = false,
  onFileUploaded,
}) => {
  const [isDragging, setIsDragging] = useState(false);
  const [uploads, setUploads] = useState<Map<string, UploadState>>(new Map());
  const [successToast, setSuccessToast] = useState<{ open: boolean; filename: string }>({
    open: false,
    filename: '',
  });
  const dragCounter = useRef(0);

  const handleDragEnter = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (disabled) return;
      dragCounter.current++;
      if (e.dataTransfer.items?.length > 0) {
        setIsDragging(true);
      }
    },
    [disabled]
  );

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current--;
    if (dragCounter.current === 0) {
      setIsDragging(false);
    }
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  const uploadFile = useCallback(
    async (file: File, openFileManager: boolean = true) => {
      const uploadId = `${file.name}-${Date.now()}`;

      setUploads((prev) => new Map(prev).set(uploadId, { progress: 0, name: file.name }));

      try {
        const formData = new FormData();
        formData.append('file', file);

        const xhr = new XMLHttpRequest();

        xhr.upload.addEventListener('progress', (event) => {
          if (event.lengthComputable) {
            const progress = Math.round((event.loaded / event.total) * 100);
            setUploads((prev) => new Map(prev).set(uploadId, { progress, name: file.name }));
          }
        });

        const response = await new Promise<{ path: string; filename: string }>((resolve, reject) => {
          xhr.onload = () => {
            if (xhr.status >= 200 && xhr.status < 300) {
              try {
                const data = JSON.parse(xhr.responseText);
                resolve(data);
              } catch {
                resolve({ path: '', filename: file.name });
              }
            } else {
              reject(new Error(`Upload failed: ${xhr.statusText}`));
            }
          };
          xhr.onerror = () => reject(new Error('Upload failed'));

          // Pass query param to control file manager opening
          const url = `/api/v1/external-agents/${sessionId}/upload${openFileManager ? '' : '?open_file_manager=false'}`;
          xhr.open('POST', url);
          xhr.send(formData);
        });

        // Mark as complete in progress indicator
        setUploads((prev) => new Map(prev).set(uploadId, { progress: 101, name: file.name }));

        // Show success toast
        setSuccessToast({ open: true, filename: file.name });

        // Notify parent with the uploaded file path
        if (response.path && onFileUploaded) {
          onFileUploaded(response.path);
        }

        // Remove from uploads after brief delay
        setTimeout(() => {
          setUploads((prev) => {
            const next = new Map(prev);
            next.delete(uploadId);
            return next;
          });
        }, 2000);
      } catch (error) {
        console.error('Upload failed:', error);
        // Mark as error
        setUploads((prev) => new Map(prev).set(uploadId, { progress: -1, name: file.name }));
        setTimeout(() => {
          setUploads((prev) => {
            const next = new Map(prev);
            next.delete(uploadId);
            return next;
          });
        }, 3000);
      }
    },
    [sessionId, onFileUploaded]
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setIsDragging(false);
      dragCounter.current = 0;

      if (disabled) return;

      const files = Array.from(e.dataTransfer.files);
      files.forEach((file) => uploadFile(file));
    },
    [disabled, uploadFile]
  );

  const getProgressText = (progress: number): string => {
    if (progress === -1) return 'Upload failed';
    if (progress === 101) return 'Uploaded to ~/work/incoming';
    if (progress === 100) return 'Finishing...';
    return `Uploading to ~/work/incoming... ${progress}%`;
  };

  return (
    <Box
      sx={{ position: 'relative', width: '100%', flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      {children}

      {/* Drag overlay */}
      <Fade in={isDragging}>
        <Box
          sx={{
            position: 'absolute',
            inset: 0,
            backgroundColor: 'rgba(25, 118, 210, 0.15)',
            border: '3px dashed',
            borderColor: 'primary.main',
            borderRadius: 2,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1000,
            pointerEvents: 'none',
          }}
        >
          <CloudUploadIcon sx={{ fontSize: 64, color: 'primary.main' }} />
          <Typography variant="h6" color="primary" sx={{ mt: 2 }}>
            Drop files to upload to sandbox
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Files will be saved to ~/work/incoming
          </Typography>
        </Box>
      </Fade>

      {/* Upload progress indicators */}
      {uploads.size > 0 && (
        <Box
          sx={{
            position: 'absolute',
            bottom: 16,
            right: 16,
            width: 320,
            zIndex: 1001,
            display: 'flex',
            flexDirection: 'column',
            gap: 1,
          }}
        >
          {Array.from(uploads.entries()).map(([id, { progress, name }]) => (
            <Box
              key={id}
              sx={{
                backgroundColor: 'background.paper',
                borderRadius: 1,
                p: 1.5,
                boxShadow: 3,
              }}
            >
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
                {progress === 101 && <CheckIcon sx={{ fontSize: 16, color: 'success.main' }} />}
                <Typography variant="body2" noWrap sx={{ flex: 1 }}>
                  {name}
                </Typography>
              </Box>
              <LinearProgress
                variant={progress >= 0 && progress <= 100 ? 'determinate' : 'indeterminate'}
                value={progress >= 0 && progress <= 100 ? progress : 0}
                color={progress === -1 ? 'error' : progress === 101 ? 'success' : 'primary'}
                sx={{ mb: 0.5 }}
              />
              <Typography variant="caption" color="text.secondary">
                {getProgressText(progress)}
              </Typography>
            </Box>
          ))}
        </Box>
      )}

      {/* Success toast */}
      <Snackbar
        open={successToast.open}
        autoHideDuration={3000}
        onClose={() => setSuccessToast({ open: false, filename: '' })}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          severity="success"
          variant="filled"
          onClose={() => setSuccessToast({ open: false, filename: '' })}
        >
          {successToast.filename} uploaded to ~/work/incoming
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default SandboxDropZone;
