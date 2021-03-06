const commonBuilder = require('pulito');
const { resolve } = require('path')

module.exports = (env, argv) => {
  let config = commonBuilder(env, argv, __dirname);
  config.output.publicPath='/static/';
  config.resolve = config.resolve || {};
  config.resolve.alias = config.resolve.alias || {};
  return config;
}
