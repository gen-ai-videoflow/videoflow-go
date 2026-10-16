export function historyQuery(provider,product,cursor='') {
 if(!['all','stripe','cryptorails'].includes(provider)||!['all','credits','premium'].includes(product))throw new Error('Choose a supported payment history filter.');
 const query=new URLSearchParams({provider,product,limit:'20'});
 if(cursor)query.set('cursor',cursor);
 return query;
}
export function mergeHistoryEntries(previous,next) {
 const entries=new Map(previous.map(entry=>[entry.id,entry]));
 for(const entry of next)entries.set(entry.id,entry);
 return [...entries.values()];
}
