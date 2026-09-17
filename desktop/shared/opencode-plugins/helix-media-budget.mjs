const MEDIA_BUDGET_BYTES = 6 * 1024 * 1024
const DATA_IMAGE = /^data:image\/[^;,]+;base64,[A-Za-z0-9+/]*={0,2}$/

export default async function helixMediaBudget() {
  return {
    "experimental.chat.messages.transform": async (_input, output) => {
      const seen = new Set()
      let used = 0

      for (let messageIndex = output.messages.length - 1; messageIndex >= 0; messageIndex--) {
        const parts = output.messages[messageIndex].parts
        for (let partIndex = parts.length - 1; partIndex >= 0; partIndex--) {
          const part = parts[partIndex]
          if (part.type !== "tool" || part.state.status !== "completed" || !part.state.attachments) continue

          const markers = []
          const source =
            typeof part.state.input?.filePath === "string" ? ` Source: ${JSON.stringify(part.state.input.filePath)}.` : ""
          for (let attachmentIndex = part.state.attachments.length - 1; attachmentIndex >= 0; attachmentIndex--) {
            const attachment = part.state.attachments[attachmentIndex]
            if (typeof attachment.url !== "string" || !DATA_IMAGE.test(attachment.url)) continue

            let marker
            if (seen.has(attachment.url)) {
              marker = `[Image attachment omitted: duplicate of a newer image.${source}]`
            } else {
              seen.add(attachment.url)
              if (used + attachment.url.length > MEDIA_BUDGET_BYTES) {
                marker = `[Image attachment omitted: aggregate image budget exceeded.${source}]`
              } else {
                used += attachment.url.length
              }
            }
            if (!marker) continue

            part.state.attachments.splice(attachmentIndex, 1)
            markers.push(marker)
          }
          if (markers.length > 0) {
            part.state.output += `${part.state.output ? "\n" : ""}${markers.join("\n")}`
          }
        }
      }
    },
  }
}
