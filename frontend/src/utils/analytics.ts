export const emitEvent = ({
  name,
}: {
  name: string,
}) => {
  const win = (window as any)
  if(!win.dataLayer) return
  win.dataLayer.push({'event':name})
}

export const reportError = (err: any) => {
  const win = (window as any)
  if(!win.emitError) return
  win.emitError(err)
}