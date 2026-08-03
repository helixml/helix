import { FC } from 'react'
import FilterListIcon from '@mui/icons-material/FilterList'

import { ChartTopicFilter, CHART_TOPIC_FILTERS } from './chartTopicVisibility'
import ChartVisibilityMenu from './ChartVisibilityMenu'

const ChartTopicVisibilityMenu: FC<{
  selected: ChartTopicFilter[]
  onChange: (selected: ChartTopicFilter[]) => void
}> = ({ selected, onChange }) => {
  return (
    <ChartVisibilityMenu
      label="Topics"
      icon={<FilterListIcon />}
      options={CHART_TOPIC_FILTERS}
      selected={selected}
      onChange={(filters) => onChange(filters as ChartTopicFilter[])}
      allLabel="All topic types"
    />
  )
}

export default ChartTopicVisibilityMenu
