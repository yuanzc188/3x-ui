export type ForwardDestType = 'socks' | 'http';

export interface ForwardRule {
  id: number;
  inboundTag: string;
  remark?: string;
  destType: ForwardDestType;
  destAddress: string;
  destPort: number;
  username?: string;
  password?: string;
  enable: boolean;
  // Domain whitelist: off → unrestricted; on + empty domains → global; on + domains → custom.
  domainLimit: boolean;
  domains?: string;
  // Provider-side proxy expiry (ms), 0 = unset.
  expiryTime: number;
  // Health-check results (server-owned, read-only in the UI).
  checkedAt: number;
  checkOk: boolean;
  checkIp?: string;
  checkGeo?: string;
  checkMs: number;
  checkErr?: string;
  // List-only: bound inbound has sniffing disabled, whitelist can't match.
  sniffingOff?: boolean;
}

export interface ForwardSettings {
  globalDomains: string;
  checkUrl: string;
}
