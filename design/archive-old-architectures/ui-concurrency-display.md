# UI Concurrency Display Implementation

This document describes the frontend UI implementation for displaying concurrent request processing information in the Helix dashboard.

## Overview

The Helix dashboard now displays real-time concurrency metrics for each model instance, showing how many requests are currently being processed concurrently and the maximum capacity for each slot.

## UI Components Modified

### 1. TypeScript API Types (`frontend/src/api/api.ts`)

Added new fields to the `TypesRunnerSlot` interface:

```typescript
export interface TypesRunnerSlot {
    // ... existing fields
    active_requests?: number;     // Current number of concurrent requests
    max_concurrency?: number;     // Maximum concurrent requests allowed
    // ... existing fields
}
```

Also added concurrency field to `TypesModel` interface:

```typescript
export interface TypesModel {
    // ... existing fields
    concurrency?: number;         // Per-model concurrency override
    // ... existing fields
}
```

### 2. ModelInstanceSummary Component (`frontend/src/components/session/ModelInstanceSummary.tsx`)

Enhanced the component to display concurrent request information:

#### Visual Implementation
- **Display Format**: `X/Y requests` (e.g., "3/8 requests")
- **Visibility**: Only shown for VLLM and Ollama runtimes
- **Styling**: Compact badge with blue theme to match Helix design
- **Positioning**: Adjacent to the status indicator

#### Styling Details
```typescript
sx={{
    color: "rgba(255, 255, 255, 0.7)",
    fontSize: "0.7rem",
    px: 1,
    py: 0.3,
    borderRadius: "2px",
    backgroundColor: "rgba(0, 200, 255, 0.1)",
    border: "1px solid rgba(0, 200, 255, 0.2)",
    fontFamily: "monospace",
    cursor: "help",
}}
```

#### Interactive Tooltip
Provides detailed explanation when users hover:
> "Currently processing X out of Y maximum concurrent requests. This model instance can handle multiple requests simultaneously for better throughput."

### 3. Data Flow Integration

#### Backend Integration
- **API Endpoint**: `/api/v1/dashboard` returns enriched slot data
- **Update Frequency**: 1-second refresh interval (matches backend reconciliation)
- **Data Source**: Scheduler enriches runner slot data with concurrency metrics

#### State Management
- Uses React Query for automatic data fetching and caching
- Real-time updates via `refetchInterval: 1000`
- Consistent with existing dashboard update patterns

## User Experience

### Visual Indicators

#### Active Processing
```
┌─────────────────────────────────┐
│ llama3:8b-instruct [Ollama]     │
│ ● Ready    [3/4 requests]       │
│ 8.5GB                           │
└─────────────────────────────────┘
```

#### At Capacity
```
┌─────────────────────────────────┐
│ qwen-vl-7b [VLLM]              │
│ ● Ready    [12/12 requests]     │
│ 39GB                            │
└─────────────────────────────────┘
```

#### Single Request Models
```
┌─────────────────────────────────┐
│ custom-model [Diffusers]        │
│ ● Ready                         │
│ 5.2GB                           │
└─────────────────────────────────┘
```

### Progressive Disclosure

1. **Primary Information**: Model name, status, memory
2. **Secondary Information**: Concurrency metrics (when applicable)
3. **Detailed Information**: Tooltip with full explanation

### Runtime-Specific Behavior

| Runtime | Display Logic | Example |
|---------|--------------|---------|
| **VLLM** | Always show if `active_requests` and `max_concurrency` are available | `3/256 requests` |
| **Ollama** | Always show if `active_requests` and `max_concurrency` are available | `2/4 requests` |
| **Others** | Hidden (no concurrency support) | Status only |

## Technical Implementation

### Conditional Rendering
```typescript
{(slot.runtime === "vllm" || slot.runtime === "ollama") &&
    slot.active_requests !== undefined &&
    slot.max_concurrency !== undefined && (
    // Render concurrency information
)}
```

### Accessibility Features
- **Semantic HTML**: Uses proper Typography components
- **Keyboard Navigation**: Tooltip accessible via focus
- **Screen Readers**: Clear aria-labels and descriptive text
- **Color Contrast**: High contrast blue theme for visibility

### Responsive Design
- **Compact Layout**: Minimal space usage in dense dashboard
- **Mobile Friendly**: Readable at small sizes
- **Scalable**: Works with various slot counts per runner

## Dashboard Integration

### Real-Time Updates
- **Automatic Refresh**: Updates every second via React Query
- **Live Metrics**: Shows current active request counts
- **Immediate Feedback**: Users see concurrency changes in real-time

### Performance Considerations
- **Minimal Overhead**: Only renders when data is available
- **Efficient Updates**: React Query handles caching and deduplication
- **No Additional API Calls**: Uses existing dashboard endpoint

## Benefits for Users

### Operational Visibility
1. **Capacity Monitoring**: See which model instances are busy
2. **Load Distribution**: Identify underutilized slots
3. **Performance Tuning**: Understand concurrency utilization patterns

### Troubleshooting Support
1. **Queue Analysis**: Correlate high queue times with slot capacity
2. **Configuration Validation**: Verify concurrency settings are applied
3. **Resource Planning**: Plan capacity based on observed utilization

### Performance Optimization
1. **Bottleneck Identification**: Find models hitting capacity limits
2. **Scaling Decisions**: Data-driven scaling recommendations
3. **Configuration Tuning**: Adjust per-model concurrency based on usage

## Future Enhancements

### Planned Improvements
1. **Historical Charts**: Show concurrency utilization over time
2. **Capacity Alerts**: Visual warnings when approaching limits
3. **Efficiency Metrics**: Request throughput and latency correlation
4. **Configuration Tools**: In-UI concurrency adjustment

### Integration Opportunities
- **Grafana Dashboards**: Export concurrency metrics
- **Alert Systems**: Notify on capacity thresholds
- **Auto-scaling**: Trigger based on utilization patterns

This implementation provides immediate operational visibility into concurrent request processing while maintaining a clean, intuitive user interface that scales with the complexity of the deployment.