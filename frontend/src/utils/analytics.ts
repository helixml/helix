export const emitEvent = ({
  name,
}: {
  name: string,
}) => {
  const win = (window as any)
  if(!win.dataLayer) return
  win.dataLayer.push({'event':name})
}