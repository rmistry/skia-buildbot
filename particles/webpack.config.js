const commonBuilder = require('pulito');
const CopyWebpackPlugin = require('copy-webpack-plugin')
const { resolve } = require('path')

module.exports = (env, argv) => {
  let config = commonBuilder(env, argv, __dirname);
  config.output.publicPath='/static/';
  config.plugins.push(
    new CopyWebpackPlugin([{
      from: './node_modules/jsoneditor/dist/jsoneditor.min.css',
      to: 'jsoneditor.css'
    },{
      from: './node_modules/jsoneditor/dist/img/jsoneditor-icons.svg',
      to: 'img/jsoneditor-icons.svg'
    },{ from: 'build/canvaskit/canvaskit.wasm' },
      { from: 'node_modules/@webcomponents/custom-elements/custom-elements.min.js' }
    ])
  );
  config.node = {
    fs: 'empty'
  };
  return config;
}
