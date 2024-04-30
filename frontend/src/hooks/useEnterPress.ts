import { useCallback } from 'react'

type IUpdateHandler = (value: string) => void
type ITriggerHandler = () => void

export const useEnterPress = ({
  value,
  updateHandler,
  triggerHandler,
}: {
  value: string,
  updateHandler: IUpdateHandler,
  triggerHandler: ITriggerHandler,
}) => {
  
  // this should be attached to the onKeyDown of the input field
  const handler = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      if (event.shiftKey) {
        updateHandler(value + "\n")
      } else {
        triggerHandler()
      }
    }
  }, [
    value,
  ])

  return handler
}

export default useEnterPress