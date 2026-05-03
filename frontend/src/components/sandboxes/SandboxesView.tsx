import { FC } from 'react'

import CardGrid from '../widgets/CardGrid'
import { ViewMode } from '../widgets/ViewModeToggle'
import SandboxCard from './SandboxCard'
import SandboxesTable from './SandboxesTable'
import { TypesSandbox } from '../../api/api'

interface SandboxesViewProps {
  mode: ViewMode
  sandboxes: TypesSandbox[]
  onOpen: (sandbox: TypesSandbox) => void
  onDelete: (sandbox: TypesSandbox) => void
  orgId: string
}

const SandboxesView: FC<SandboxesViewProps> = ({ mode, sandboxes, onOpen, onDelete, orgId }) => {
  if (mode === 'cards') {
    return (
      <CardGrid
        items={sandboxes}
        getKey={(sb) => sb.id ?? ''}
        renderCard={(sb) => <SandboxCard sandbox={sb} onOpen={onOpen} onDelete={onDelete} orgId={orgId} />}
      />
    )
  }
  return <SandboxesTable sandboxes={sandboxes} onOpen={onOpen} onDelete={onDelete} />
}

export default SandboxesView
