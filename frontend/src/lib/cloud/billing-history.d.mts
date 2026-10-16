export function historyQuery(provider:string,product:string,cursor?:string):URLSearchParams;
export function mergeHistoryEntries<T extends {id:string}>(previous:T[],next:T[]):T[];
