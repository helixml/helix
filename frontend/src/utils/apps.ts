import {
  IApp,
} from '../types'

export const getAppImage = (app: IApp): string => {
  if(app.config.helix?.image) {
    return app.config.helix?.image
  }

  if(app.config.helix?.assistants?.length) {
    return app.config.helix?.assistants[0].image
  }

  return ''
}

export const getAppAvatar = (app: IApp): string => {
  if(app.config.helix?.avatar) {
    return app.config.helix?.avatar
  }

  if(app.config.helix?.assistants?.length) {
    return app.config.helix?.assistants[0].avatar
  }

  return ''
}

export const getAppName = (app: IApp): string => {
  if(app.config.helix?.name) {
    return app.config.helix?.name
  }

  if(app.config.helix?.assistants?.length) {
    return app.config.helix?.assistants[0].name
  }

  return ''
}

export const getAppDescription = (app: IApp): string => {
  if(app.config.helix?.description) {
    return app.config.helix?.description
  }

  if(app.config.helix?.assistants?.length) {
    return app.config.helix?.assistants[0].description
  }

  return ''
}