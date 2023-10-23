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

export interface ISession {
  id: string;
  name: string;
  created: Date;
  updated: Date;
  mode: string;
  type: string;
  model_name: string;
  finetune_file: string;
  interactions: IInteractions;
  owner: string;
  owner_type: IOwnerType;
}

export interface IInteractions {
  [key: string]: any;
}

export type IOwnerType = 'user' | 'system' | 'org';

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