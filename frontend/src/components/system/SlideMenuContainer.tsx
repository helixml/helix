import React, { FC, ReactNode } from 'react'

interface SlideMenuContainerProps {
  children: ReactNode;
  menuType: string; // Identifier for the menu type
}

const SlideMenuContainer: FC<SlideMenuContainerProps> = ({ 
  children,
  menuType
}) => {
  return (
    <div 
      style={{
        width: '100%',
        minHeight: 'fit-content', // Allow natural content height
        overflow: 'visible', // Let content contribute to parent height
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {children}
    </div>
  )
}

export default SlideMenuContainer 