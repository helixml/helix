export interface IUser {
  id: string,
  email: string,
  token: string,
}

export interface IMachineSpec {
  gpu: number,
  cpu: number,
  ram: number,
}

export interface IModuleConfig {
  name: string,
  repo: string,
  hash: string,
  path: string,
}

export interface IJobOffer {
  id: string,
  created_at: number,
  job_creator: string,
  module: IModuleConfig,
  spec: IMachineSpec,
  inputs: Record<string, string>,
  mode: string,
}

export interface IJobContainer {
  id: string,
  deal_id: string,
  job_creator: string,
  state: number,
  job_offer: IJobOffer,
}

export interface IJobSpec {
  module: string,
  inputs: Record<string, string>,
}

export interface IJobData {
  spec: IJobSpec,
  container: IJobContainer,
}

export interface IJob {
  id: string,
  created: number,
  owner: string,
  owner_type: string,
  state: string,
  status: string,
  data: IJobData,
}

export interface IWebsocketEvent {
  type: string,
  job?: IJob,
}

export interface IModule {
  id: string,
  title: string,
  cost: number,
  template: string,
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