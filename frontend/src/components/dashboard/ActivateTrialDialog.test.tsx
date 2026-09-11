import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ActivateTrialDialog from './ActivateTrialDialog';

const mocks = vi.hoisted(() => ({
    ownedOrgs: [] as Array<{ id: string; name: string; display_name?: string }>,
    mutateAsync: vi.fn(),
}));

vi.mock('../../services/dashboardService', () => ({
    useAdminActivateTrial: () => ({ mutateAsync: mocks.mutateAsync, isPending: false }),
    useAdminUserOwnedOrgs: () => ({ data: mocks.ownedOrgs, isLoading: false }),
}));

vi.mock('../../hooks/useSnackbar', () => ({
    default: () => ({ success: vi.fn() }),
}));

const user = { id: 'user-1', email: 'user@example.com' };

describe('ActivateTrialDialog', () => {
    beforeEach(() => {
        mocks.ownedOrgs = [];
        mocks.mutateAsync.mockReset().mockResolvedValue({ status: 'applied' });
    });

    it('keeps a manual org selection when owned org data refetches', async () => {
        mocks.ownedOrgs = [
            { id: 'org-a', name: 'Alpha' },
            { id: 'org-b', name: 'Beta' },
        ];
        const { rerender } = render(<ActivateTrialDialog open onClose={vi.fn()} user={user} />);

        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Organisation' }));
        fireEvent.click(screen.getByRole('option', { name: 'Beta (org-b)' }));

        mocks.ownedOrgs = [
            { id: 'org-c', name: 'Gamma' },
            { id: 'org-b', name: 'Beta' },
            { id: 'org-a', name: 'Alpha' },
        ];
        rerender(<ActivateTrialDialog open onClose={vi.fn()} user={user} />);
        fireEvent.click(screen.getByRole('button', { name: 'Activate trial' }));

        await waitFor(() => expect(mocks.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({ orgId: 'org-b' })));
    });

    it('submits the sole owned org by default', async () => {
        mocks.ownedOrgs = [{ id: 'org-a', name: 'Alpha' }];
        render(<ActivateTrialDialog open onClose={vi.fn()} user={user} />);

        fireEvent.click(screen.getByRole('button', { name: 'Activate trial' }));

        await waitFor(() => expect(mocks.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({ orgId: 'org-a' })));
    });
});
