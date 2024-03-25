import { FC, createElement, useState } from 'react'
import { styled, setup } from 'goober'

import Search, { SearchBoxTheme } from './components/Search'
import Modal from './components/Modal'

setup(createElement)

const SearchContainer = styled("div")`
  background-color: #070714;
  padding: 200px;
`;

const Widget: FC<{
  searchBoxTheme?: SearchBoxTheme,
}> = ({
  searchBoxTheme = {},
}) => {
  const [ open, setOpen ] = useState(false)

  return (
    <>
      <SearchContainer>
        <Search
          onClick={ () => setOpen(true) }
          { ...searchBoxTheme }
        />
        <Modal
          open={ open }
          onClose={ () => {
            setOpen(false)
          }}
        />
      </SearchContainer>
    </>
  )
}

export default Widget
