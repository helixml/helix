import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ActivateTrialDialog from './ActivateTrialDialog';

const mocks = vi.hoisted(() => ({
    ownedOrgs: [] as Array<{ id: string; name: string; display_name?: string }>,
    mutateAsync: vi.fn(),
}));

vi.mock('../../services/dashboardService', () => ({
    useAdminActivateTrial: () => ({ mutateAsync: mocks.mutateAsync, isPending: false }),
    useAdminRevokeTrial: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useAdminUserOwnedOrgs: () => ({ data: mocks.ownedOrgs, isLoading: false }),
}));

vi.mock('../../hooks/useSnackbar', () => ({
    default: () => ({ success: vi.fn() }),
}));

const user = { id: 'user-1', email: 'user@example.com' };

describe('ActivateTrialDialog', () => {
    beforeEach(() => {
        mocks.ownedOrgs = [];
        mocks.mutateAsync.mockReset().mockResolvedValue({ status: 'stashed' });
    });

    it('stashes the trial intent for a user who owns no organisations', async () => {
        render(<ActivateTrialDialog open onClose={vi.fn()} user={user} />);

        fireEvent.click(screen.getByRole('button', { name: 'Activate trial' }));

        await waitFor(() => expect(mocks.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({ orgId: undefined })));
        expect(screen.queryByText(/already owns/)).toBeNull();
    });

    it('points to the org screen and disables submit when the user owns organisations', () => {
        mocks.ownedOrgs = [
            { id: 'org-a', name: 'Alpha' },
            { id: 'org-b', name: 'Beta' },
        ];
        render(<ActivateTrialDialog open onClose={vi.fn()} user={user} />);

        expect(screen.getByText(/already owns 2 organisations/)).toBeTruthy();
        expect(screen.getByRole('button', { name: 'Activate trial' })).toBeDisabled();
        expect(mocks.mutateAsync).not.toHaveBeenCalled();
    });
});
