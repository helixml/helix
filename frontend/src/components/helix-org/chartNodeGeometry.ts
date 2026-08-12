export type ChartNodeKind = 'bot' | 'topic' | 'processor' | 'asset'

export const BOT_W = 220
export const BOT_H = 96
export const STREAM_W = 180
export const STREAM_H = 80
export const ASSET_W = 220
export const ASSET_H = 100
export const PROC_W = 220

export const PROC_HEADER_H = 50
export const PROC_ROW_H = 28

export const procNodeHeight = (outputCount: number) => PROC_HEADER_H + Math.max(1, outputCount) * PROC_ROW_H

export const centeredCreatedNodePosition = (
  kind: ChartNodeKind,
  point: { x: number; y: number },
): { x: number; y: number } => {
  const size = kind === 'bot'
    ? { width: BOT_W, height: BOT_H }
    : kind === 'topic'
      ? { width: STREAM_W, height: STREAM_H }
      : kind === 'processor'
        ? { width: PROC_W, height: procNodeHeight(1) }
        : { width: ASSET_W, height: ASSET_H }
  return { x: point.x - size.width / 2, y: point.y - size.height / 2 }
}
