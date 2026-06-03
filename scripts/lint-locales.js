#!/usr/bin/env node

const cp = require('child_process');
const { promisify } = require('util');
const pkg = require('../package.json');

// The legacy Node config (server/config.js) is gone — the backend is now in
// Go. We only used its `env` field below; read NODE_ENV directly instead.
const conf = { env: process.env.NODE_ENV || 'development' };

const exec = promisify(cp.exec);
const cmd = `compare-locales l10n.toml . ${getLocales()} --data=json`;

console.log(cmd);

exec(cmd)
  .then(({ stdout }) => JSON.parse(stdout))
  .then(({ details }) => filterErrors(details))
  .then(results => {
    if (results.length) {
      results.forEach(({ locale, data }) => {
        console.log(locale);
        data.forEach(msg => console.log(`- ${msg}`));
        console.log('');
      });
      process.exit(2);
    }
  })
  .catch(err => {
    console.error(err);
    process.exit(1);
  });

function filterErrors(details) {
  return Object.keys(details)
    .sort()
    .map(locale => {
      const data = details[locale]
        .filter(item => Object.prototype.hasOwnProperty.call(item, 'error'))
        .map(({ error }) => error);
      return { locale, data };
    })
    .filter(({ data }) => data.length);
}

function getLocales() {
  // If we're in a "production" env (or passed the `--production` flag), only
  // check the locales from the package.json file's `availableLanguages` array.
  if (conf.env === 'production' || process.argv.includes('--production')) {
    return pkg.availableLanguages.sort().join(' ');
  }
  // Lint all the locales.
  return '`ls public/locales`';
}
