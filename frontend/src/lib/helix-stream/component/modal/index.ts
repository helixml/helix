// Stub implementation - modals not needed in React integration
export async function showMessage(message: string): Promise<void> {
  console.log('[HelixStream Message]:', message);
}

export async function showModal(modal: any): Promise<any> {
  // For credentials, we'll handle this differently in React
  return null;
}
