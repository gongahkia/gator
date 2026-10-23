import { test } from "node:test";
import assert from "node:assert/strict";
import { nativeAppearance } from "../electron/appearance.mjs";
import { readFileSync } from "node:fs";

test("window chrome follows the profile's light or dark canvas",()=>{
 assert.deepEqual(nativeAppearance("light"),{themeSource:"light",background:"#fafbf8"});
 for(const theme of ["gator","mono","contrast",undefined,"unknown"])assert.deepEqual(nativeAppearance(theme),{themeSource:"dark",background:"#101010"});
});

test("light theme small text and primary controls meet normal text contrast",()=>{
 const css=readFileSync(new URL("../src/styles.css",import.meta.url),"utf8");
 const light=css.match(/:root\[data-theme="light"\], \.app-shell\[data-theme="light"\] \{([^}]+)\}/)[1];
 const colors=Object.fromEntries([...light.matchAll(/--(color-[\w-]+): (#[a-f0-9]{6});/g)].map(([,key,value])=>[key,value]));
 const luminance=hex=>{const rgb=hex.slice(1).match(/../g).map(x=>parseInt(x,16)/255).map(x=>x<=.04045?x/12.92:((x+.055)/1.055)**2.4);return rgb[0]*.2126+rgb[1]*.7152+rgb[2]*.0722;};
 const ratio=(a,b)=>{const x=luminance(a),y=luminance(b);return(Math.max(x,y)+.05)/(Math.min(x,y)+.05);};
 for(const ink of ["contrast","muted","secondary","faint","accent","warning","danger"]){
  for(const canvas of ["base","sidebar","surface","raised","hover"])assert.ok(ratio(colors[`color-${ink}`],colors[`color-${canvas}`])>=4.5,`${ink} on ${canvas} has insufficient contrast`);
 }
 for(const button of ["accent","accent-hover"])assert.ok(ratio(colors["color-on-accent"],colors[`color-${button}`])>=4.5,`white text on ${button} has insufficient contrast`);
});
