/**
 * Frontend Instance ID
 *
 * Generated once when the app loads and persists for the entire browser session.
 * Used to differentiate multiple browser tabs/windows viewing the same resources.
 *
 * Example use case: Multiple tabs streaming the same Moonlight session need unique
 * client identifiers to avoid conflicts.
 */

// Generate a simple UUID v4
function generateUUID(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
}

// Generate instance ID once at module load time
export const FRONTEND_INSTANCE_ID = generateUUID();

console.log('[InstanceID] Frontend instance initialized:', FRONTEND_INSTANCE_ID);
