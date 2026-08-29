import { expect, test } from "@playwright/test";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";

test("renders a multi-dashboard report without network access", async ({ page }, testInfo) => {
  const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),"..");
  const runtime=await readFile(path.join(root,"dist","report","report.js"),"utf8");
  const styles=await readFile(path.join(root,"dist","report","report.css"),"utf8");
  const generatedAt="2026-08-29T12:00:00Z";
  const manifest={schema_version:1,jobdock_version:"v-test",generated_at:generatedAt,job:{id:"job-1",name:"Offline training report",status:"SUCCEEDED",created_at:generatedAt,finished_at:generatedAt},attempt:{id:"attempt-1",attempt_number:3,status:"SUCCEEDED",created_at:generatedAt,finished_at:generatedAt},dashboards:[
    {id:"overview",name:"Training overview",schema_version:1,updated_at:generatedAt,config:{appearance:{schema_version:1,palette:{id:"ocean",version:1}},widgets:[{id:"loss",type:"lineplot",title:"Loss",size:{columns:8,rows:5},position:{x:0,y:0},sources:[{kind:"metric",name:"loss"}],x_axis:"step"},{id:"progress",type:"progress",size:{columns:4,rows:5},position:{x:8,y:0},sources:[{kind:"progress",name:"progress"}]}]}},
    {id:"quality",name:"Quality",schema_version:1,updated_at:generatedAt,config:{appearance:{schema_version:1,palette:{id:"forest",version:1}},widgets:[{id:"confusion",type:"confusion_matrix",title:"Confusion matrix",size:{columns:7,rows:6},position:{x:0,y:0},sources:[{kind:"matrix",name:"confusion"}]},{id:"logs",type:"logs",title:"Logs",size:{columns:5,rows:6},position:{x:7,y:0},sources:[{kind:"log",name:"stdout"}]}]}}
  ],sources:{metrics:[{name:"loss",unit:"ratio",points:[{captured_at:"2026-08-29T11:59:00Z",step:1,value:.8,sample_count:1},{captured_at:"2026-08-29T12:00:00Z",step:2,value:.4,sample_count:1}],last:.4,min:.4,max:.8,avg:.6,sample_count:2}],resources:[],matrices:{confusion:{name:"confusion",values:[[18,2],[1,19]],row_labels:["actual 0","actual 1"],column_labels:["predicted 0","predicted 1"]}},distributions:{},tables:{},logs:{stdout:"training complete\naccuracy=0.925"},progress:{global_progress:1},checkpoints:[]},warnings:[]};
  const encoded=Buffer.from(JSON.stringify(manifest)).toString("base64");
  const safeRuntime=runtime.replaceAll("</script","<\\/script");
  const hash=createHash("sha256").update(safeRuntime).digest("base64");
  const csp=`default-src 'none'; script-src 'sha256-${hash}'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'`;
  const html=`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}"><style>${styles}</style></head><body><div id="root"></div><script id="jobdock-report-data" type="application/json">${encoded}</script><script>${safeRuntime}</script></body></html>`;
  const reportPath=testInfo.outputPath("jobdock-report.html");
  await writeFile(reportPath,html);
  const externalRequests:string[]=[],runtimeErrors:string[]=[];
  page.on("request",request=>{if(/^https?:/.test(request.url()))externalRequests.push(request.url())});
  page.on("console",message=>{if(message.type()==="error")runtimeErrors.push(message.text())});
  page.on("pageerror",error=>runtimeErrors.push(error.message));
  await page.goto(pathToFileURL(reportPath).href);
  expect(runtimeErrors).toEqual([]);
  await expect(page.getByRole("heading",{name:"Offline training report"})).toBeVisible();
  await expect(page.getByRole("img",{name:"Loss"})).toBeVisible();
  await expect(page.getByText("loss (ratio)")).toBeVisible();
  await page.getByRole("button",{name:"Quality"}).click();
  await expect(page.getByText("accuracy=0.925")).toBeVisible();
  await expect(page.locator(".matrix-cell")).toHaveCount(4);
  await expect(page.getByText(/Edit dashboard|Rerun|Delete job/)).toHaveCount(0);
  expect(externalRequests).toEqual([]);
});
