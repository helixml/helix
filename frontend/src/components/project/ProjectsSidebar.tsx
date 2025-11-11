import React, { FC } from 'react'
import { Kanban, GitBranch } from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

const ProjectsSidebar: FC = () => {
  const router = useRouter()
  const { tab } = router.params
  const currentView = tab || 'projects'

  const handleNavigationClick = (tabValue: string) => {
    router.setParams({ tab: tabValue })
  }

  const sections: ContextSidebarSection[] = [
    {
      items: [
        {
          id: 'projects',
          label: 'Boards',
          icon: <Kanban size={18} />,
          isActive: currentView === 'projects',
          onClick: () => handleNavigationClick('projects')
        },
        {
          id: 'repositories',
          label: 'Repositories',
          icon: <GitBranch size={18} />,
          isActive: currentView === 'repositories',
          onClick: () => handleNavigationClick('repositories')
        }
      ]
    }
  ]

  return (
    <ContextSidebar
      menuType="projects"
      sections={sections}
    />
  )
}

export default ProjectsSidebar
