// Shared sessionStorage helpers for tracking which spec tasks have had their
// review auto-opened. Used by SpecTaskDetailContent (to guard the auto-open
// useEffect) and SpecTaskReviewPage (to mark the task on mount so that
// navigating back to the task detail never re-triggers the auto-open).

export const AUTO_OPENED_KEY = "helix_auto_opened_spec_tasks";

export const getAutoOpenedSpecTasks = (): Set<string> =>
  new Set(JSON.parse(sessionStorage.getItem(AUTO_OPENED_KEY) || "[]"));

export const addAutoOpenedSpecTask = (id: string): void => {
  const set = getAutoOpenedSpecTasks();
  set.add(id);
  sessionStorage.setItem(AUTO_OPENED_KEY, JSON.stringify([...set]));
};
