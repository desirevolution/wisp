import { readdirSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
for (const name of readdirSync('tests').filter(name=>name.endsWith('.dom.mjs')).sort()) {
  console.log(`\nDOM: ${name}`);
  const result=spawnSync(process.execPath,['--conditions=browser',`tests/${name}`],{stdio:'inherit',env:process.env});
  if(result.error) throw result.error;
  if(result.status!==0) process.exit(result.status||1);
}
