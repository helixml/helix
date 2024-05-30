import {
  IApp,
} from '../types'

export const getAppImage = (app: IApp): string => {
  if(app.config.helix?.assistants?.length == 1) {
    return app.config.helix?.assistants[0].image || app.config.helix?.image || ''
  } else {
    return app.config.helix?.image || ''
  }
}

export const getAppAvatar = (app: IApp): string => {
  if(app.config.helix?.assistants?.length == 1) {
    return app.config.helix?.assistants[0].avatar || app.config.helix?.avatar || ''
  } else {
    return app.config.helix?.avatar || ''
  }
}

export const getAppName = (app: IApp): string => {
  if(app.config.helix?.assistants?.length == 1) {
    return app.config.helix?.assistants[0].name || app.config.helix?.name || ''
  } else {
    return app.config.helix?.name || ''
  }
}

export const getAppDescription = (app: IApp): string => {
  if(app.config.helix?.assistants?.length == 1) {
    return app.config.helix?.assistants[0].description || app.config.helix?.description || ''
  } else {
    return app.config.helix?.description || ''
  }
}