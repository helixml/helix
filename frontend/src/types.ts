export interface IUser {
  id: string,
  email: string,
  token: string,
}

export interface IWebsocketEvent {
  type: string,
}

export interface IBalanceTransferData {
  job_id?: string,
  stripe_payment_id?: string,
}

export interface IBalanceTransfer {
  id: string,
  created: number,
  owner: string,
  owner_type: string,
  payment_type: string,
  amount: number,
  data: IBalanceTransferData,
}

export type IOwnerType = 'user' | 'system' | 'org';

export interface IApiKey {
  owner: string;
  owner_type: string;
  key: string;
  name: string;
}

export interface IFileStoreBreadcrumb {
  path: string,
  title: string,
}

export interface IFileStoreItem {
  created: number;
  size: number;
  directory: boolean;
  name: string;
  path: string;
  url: string;
}

export interface IFileStoreFolder {
  name: string,
  readonly: boolean,
}

export interface IFileStoreConfig {
  user_prefix: string,
  folders: IFileStoreFolder[],
}

export type ISessionCreator = 'system' | 'user'

export const SESSION_CREATOR_SYSTEM: ISessionCreator = 'system'
export const SESSION_CREATOR_USER: ISessionCreator = 'user'

export type ISessionMode = 'inference' | 'finetune'

export const SESSION_MODE_INFERENCE: ISessionMode = 'inference'
export const SESSION_MODE_FINETUNE: ISessionMode = 'finetune'

export type ISessionType = 'text' | 'image'

export const SESSION_TYPE_TEXT: ISessionType = 'text'
export const SESSION_TYPE_IMAGE: ISessionType = 'image'

export interface IInteraction {
  id: string,
  created: number,
  creator: ISessionCreator,
  runner: string,
  message: string,
  progress: number,
  files: string[],
  finetune_file: string,
  finished: boolean,
  metadata: Record<string, string>,
  error: string,
}

export interface ISession {
  id: string,
  name: string,
  created: number,
  updated: number,
  mode: ISessionMode,
  type: ISessionType,
  model_name: string,
  finetune_file: string,
  interactions: IInteraction[],
  owner: string,
  owner_type: IOwnerType,
}

export interface IServerConfig {
  filestore_prefix: string,
}