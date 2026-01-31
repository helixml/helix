import { format } from "@chbphone55/pretty-bytes";

export const formatFloat = (num: number | undefined): number => {
    if (num === undefined) {
        return 0;
    }
    return Math.round(num * 100) / 100;
};

export const subtractFloat = (
    a: number | undefined,
    b: number | undefined,
): number => {
    const useA = a || 0;
    const useB = b || 0;
    return formatFloat(useA - useB);
};

export const prettyBytes = (a: number): string => {
    const bs = format(a);
    // 1032192 → ['1,008', 'KiB', 'kibibytes']
    return `${bs[0]} ${bs[1]}`;
};

/**
 * Format megabytes to human-readable format (MB or GB)
 * @param mb Number in megabytes
 * @returns Formatted string like "500 MB" or "2.5 GB"
 */
export const formatMB = (mb: number): string => {
    if (mb === 0) return "0 MB";
    if (mb >= 1024) {
        return (mb / 1024).toFixed(1) + " GB";
    }
    return mb.toFixed(0) + " MB";
};

/**
 * Format a date string to a human-readable format
 * @param dateString ISO date string
 * @returns Formatted date string
 */
export const formatDate = (dateString: string): string => {
    if (!dateString) return "";

    const date = new Date(dateString);
    if (isNaN(date.getTime())) return "";

    const options: Intl.DateTimeFormatOptions = {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
    };

    return new Intl.DateTimeFormat("en-US", options).format(date);
};

/**
 * Generate a YAML filename from an app name
 * Converts "David - Financial Sentiment Analyst" to "david-financial-sentiment-analyst.yaml"
 * @param appName The app name to convert
 * @returns YAML filename
 */
export const generateYamlFilename = (appName: string): string => {
    if (!appName || typeof appName !== "string") return "app.yaml";

    return appName
        .toLowerCase()
        .replace(/[^a-z0-9\s-]/g, "") // Remove special characters except spaces and hyphens
        .replace(/\s+/g, "-") // Replace spaces with hyphens
        .replace(/-+/g, "-") // Replace multiple hyphens with single hyphen
        .replace(/^-|-$/g, "") // Remove leading/trailing hyphens
        .concat(".yaml");
};

/**
 * Format a date to relative time (e.g., "2 hours ago", "3 days ago")
 * @param dateString ISO date string
 * @returns Relative time string
 */
export const formatRelativeTime = (dateString: string): string => {
    if (!dateString) return "";

    const date = new Date(dateString);
    if (isNaN(date.getTime())) return "";

    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffSeconds = Math.floor(diffMs / 1000);
    const diffMinutes = Math.floor(diffSeconds / 60);
    const diffHours = Math.floor(diffMinutes / 60);
    const diffDays = Math.floor(diffHours / 24);

    if (diffSeconds < 60) {
        return "just now";
    } else if (diffMinutes < 60) {
        return `${diffMinutes} minute${diffMinutes === 1 ? "" : "s"} ago`;
    } else if (diffHours < 24) {
        return `${diffHours} hour${diffHours === 1 ? "" : "s"} ago`;
    } else if (diffDays < 7) {
        return `${diffDays} day${diffDays === 1 ? "" : "s"} ago`;
    } else {
        const diffWeeks = Math.floor(diffDays / 7);
        if (diffWeeks < 4) {
            return `${diffWeeks} week${diffWeeks === 1 ? "" : "s"} ago`;
        } else {
            const diffMonths = Math.floor(diffDays / 30);
            if (diffMonths < 12) {
                return `${diffMonths} month${diffMonths === 1 ? "" : "s"} ago`;
            } else {
                const diffYears = Math.floor(diffDays / 365);
                return `${diffYears} year${diffYears === 1 ? "" : "s"} ago`;
            }
        }
    }
};
