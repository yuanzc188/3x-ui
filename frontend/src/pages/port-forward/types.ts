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
}
