import { ReactNode } from 'react'
import Box from '@mui/material/Box'

interface CardGridProps<T> {
  items: T[]
  getKey: (item: T) => string
  renderCard: (item: T) => ReactNode
}

// CardGrid is the shared responsive grid used across list pages that switch
// between a table and a card view. Uses CSS grid so it aligns flush with the
// surrounding container — no negative margins from MUI Grid spacing.
function CardGrid<T>({ items, getKey, renderCard }: CardGridProps<T>) {
  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: {
          xs: '1fr',
          sm: 'repeat(2, 1fr)',
          lg: 'repeat(3, 1fr)',
        },
        gap: { xs: 2, sm: 3 },
        width: '100%',
      }}
    >
      {items.map((item) => (
        <Box key={getKey(item)} sx={{ minWidth: 0 }}>
          {renderCard(item)}
        </Box>
      ))}
    </Box>
  )
}

export default CardGrid
