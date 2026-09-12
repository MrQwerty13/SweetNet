import SwaggerParser from '@apidevtools/swagger-parser';
import { readFile, writeFile } from 'node:fs/promises';
import YAML from 'yaml';
const file = new URL('../api/openapi.yaml', import.meta.url);
const raw = await readFile(file, 'utf8');
if (raw.trimStart().startsWith('{')) await writeFile(file, YAML.stringify(JSON.parse(raw)));
const spec = await SwaggerParser.validate(file.pathname);
console.log(`OpenAPI ${spec.openapi}: ${Object.keys(spec.paths).length} paths validated`);
