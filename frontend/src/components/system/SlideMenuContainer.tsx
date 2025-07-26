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
        height: '100%',
      }}
    >
      {children}
    </div>
  )
}

export default SlideMenuContainer 