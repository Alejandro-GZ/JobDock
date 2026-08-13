const archivePattern = /\.(zip|tar\.gz|tgz)$/i;

export function validateBuildSource(name:string,file?:File|null){
  const trimmed=name.trim();
  if(trimmed.length<3||trimmed.length>120)return "Build name must contain between 3 and 120 characters.";
  if(!file)return "Select a project archive.";
  if(!archivePattern.test(file.name))return "Project source must be a .zip, .tar.gz, or .tgz archive.";
  if(file.size===0)return "Project archive cannot be empty.";
  return "";
}

export type BuildMode="RAILPACK"|"DOCKERFILE";
export type DockerfileConfig={contextPath:string;dockerfilePath:string};

export function buildFormData(name:string,file:File,mode:BuildMode="RAILPACK",config?:DockerfileConfig){
  const body=new FormData();
  const metadata:Record<string,string>={name:name.trim(),mode};
  if(mode==="DOCKERFILE"){
    metadata.context_path=config?.contextPath.trim()||".";
    metadata.dockerfile_path=config?.dockerfilePath.trim()||"Dockerfile";
  }
  body.append("metadata",JSON.stringify(metadata));
  body.append("source",file,file.name);
  return body;
}
