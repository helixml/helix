import {
  IApp,
  IDataPrepChunkWithFilename,
  IDataPrepStats,
  IInteraction,
  IModelInstanceState,
  IPageBreadcrumb,
  ISession,
  ISessionMode,
  ISessionSummary,
  ISessionType,
  ITextDataPrepStage,
  SESSION_CREATOR_ASSISTANT,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_IMAGE,
  TEXT_DATA_PREP_DISPLAY_STAGES,
  TEXT_DATA_PREP_STAGE_NONE,
  TEXT_DATA_PREP_STAGES,
} from '../types'

import {
  getAppName,
} from './apps'

const NO_DATE = '0001-01-01T00:00:00Z'

const COLORS: Record<string, string> = {
  sdxl_inference: '#D183C9',
  sdxl_finetune: '#E3879E',
  mistral_inference: '#F4D35E',
  text_inference: '#F4D35E', // Same as mistral inference
  mistral_finetune: '#EE964B',
}

export const hasDate = (dt?: string): boolean => {
  if (!dt) return false
  return dt != NO_DATE
}

export const getUserInteraction = (session: ISession): IInteraction | undefined => {
  const userInteractions = session.interactions.filter(i => i.creator != SESSION_CREATOR_ASSISTANT)
  if (userInteractions.length <= 0) return undefined
  return userInteractions[userInteractions.length - 1]
}

export const getAssistantInteraction = (session: ISession): IInteraction | undefined => {
  const userInteractions = session.interactions.filter(i => i.creator == SESSION_CREATOR_ASSISTANT)
  if (userInteractions.length <= 0) return undefined
  return userInteractions[userInteractions.length - 1]
}

export const getFinetuneInteraction = (session: ISession): IInteraction | undefined => {
  const userInteractions = session.interactions.filter(i => {
    return i.creator == SESSION_CREATOR_ASSISTANT && i.mode == SESSION_MODE_FINETUNE
  })
  if (userInteractions.length <= 0) return undefined
  return userInteractions[userInteractions.length - 1]
}

export const hasFinishedFinetune = (session: ISession): boolean => {
  if (session.config.original_mode != SESSION_MODE_FINETUNE) return false
  const finetuneInteraction = getFinetuneInteraction(session)
  if (!finetuneInteraction) return false
  return finetuneInteraction.finished
}

export const getColor = (modelName: string, mode: ISessionMode): string => {
  // If starts with 'ollama', return inference color
  if (getModelName(modelName).indexOf('ollama') >= 0) return COLORS['text_inference']

  const key = `${getModelName(modelName)}_${mode}`
  return COLORS[key]
}

export const getModelName = (model_name: string): string => {
  if (model_name.indexOf('stabilityai') >= 0) return 'sdxl'
  if (model_name.indexOf('mistralai') >= 0) return 'mistral'
  // If has ':' in the name, it's Ollama model, need to split and keep the first part
  if (model_name.indexOf(':') >= 0) return `ollama_${model_name.split(':')[0]}`
  return ''
}

export const getHeadline = (modelName: string, mode: ISessionMode, loraDir = ''): string => {
  let loraString = ''
  if (loraDir) {
    const parts = loraDir.split('/')
    const id = parts[parts.length - 2]
    loraString = ` - ${id.split('-').pop()}`
  }
  return `${getModelName(modelName)} ${mode} ${loraString}`
}

export const getSessionHeadline = (session: ISessionSummary): string => {
  return `${getHeadline(session.model_name, session.mode, session.lora_dir)} : ${shortID(session.session_id)} : ${getTiming(session)}`
}

export const getModelInstanceNoSessionHeadline = (modelInstance: IModelInstanceState): string => {
  return `${getHeadline(modelInstance.model_name, modelInstance.mode, modelInstance.lora_dir)} : ${getModelInstanceIdleTime(modelInstance)}`
}

export const getSummaryCaption = (session: ISessionSummary): string => {
  return session.summary
}

export const getModelInstanceIdleTime = (modelInstance: IModelInstanceState): string => {
  if (!modelInstance.last_activity) return ''
  const idleFor = Date.now() - modelInstance.last_activity * 1000
  const idleForSeconds = Math.floor(idleFor / 1000)
  return `idle for ${idleForSeconds} secs, timeout is ${modelInstance.timeout} secs, stale = ${modelInstance.stale}`
}

export const shortID = (id: string): string => {
  return id.split('-').shift() || ''
}

export const getTiming = (session: ISessionSummary): string => {
  if (hasDate(session?.scheduled)) {
    const runningFor = Date.now() - new Date(session?.scheduled || '').getTime()
    const runningForSeconds = Math.floor(runningFor / 1000)
    return `${runningForSeconds} secs`
  } else if (hasDate(session?.created)) {
    const waitingFor = Date.now() - new Date(session?.created || '').getTime()
    const waitingForSeconds = Math.floor(waitingFor / 1000)
    return `${waitingForSeconds} secs`
  } else {
    return ''
  }
}

export const getSessionSummary = (session: ISession): ISessionSummary => {
  const systemInteraction = getAssistantInteraction(session)
  const userInteraction = getUserInteraction(session)
  let summary = ''
  if (session.mode == SESSION_MODE_INFERENCE) {
    summary = userInteraction?.message || ''
  } else if (session.mode == SESSION_MODE_FINETUNE) {
    summary = `fine tuning on ${userInteraction?.files.length || 0}`
  }
  return {
    session_id: session.id,
    name: session.name,
    interaction_id: systemInteraction?.id || '',
    mode: session.mode,
    type: session.type,
    model_name: session.model_name,
    owner: session.owner,
    lora_dir: session.lora_dir,
    created: systemInteraction?.created || '',
    updated: systemInteraction?.updated || '',
    scheduled: systemInteraction?.scheduled || '',
    completed: systemInteraction?.completed || '',
    summary,
  }
}

export const getTextDataPrepStageIndex = (stage: ITextDataPrepStage): number => {
  return TEXT_DATA_PREP_STAGES.indexOf(stage)
}

export const getTextDataPrepStageIndexDisplay = (stage: ITextDataPrepStage): number => {
  return TEXT_DATA_PREP_DISPLAY_STAGES.indexOf(stage)
}

export const getTextDataPrepErrors = (interaction: IInteraction): IDataPrepChunkWithFilename[] => {
  return Object.keys(interaction.data_prep_chunks || {}).reduce((acc: IDataPrepChunkWithFilename[], filename: string) => {
    const chunks = interaction.data_prep_chunks[filename]
    const errors = chunks.filter(chunk => chunk.error != '')
    if (errors.length <= 0) return acc
    return acc.concat(errors.map(error => ({ ...error, filename })))
  }, [])
}

export const getTextDataPrepStats = (interaction: IInteraction): IDataPrepStats => {
  return Object.keys(interaction.data_prep_chunks || {}).reduce((acc: IDataPrepStats, filename: string) => {
    const chunks = interaction.data_prep_chunks[filename] || []
    const errors = chunks.filter(chunk => chunk.error != '')
    const questionCount = chunks.reduce((acc: number, chunk) => acc + chunk.question_count, 0)
    return {
      total_files: acc.total_files + 1,
      total_chunks: acc.total_chunks + chunks.length,
      total_questions: acc.total_questions + questionCount,
      converted: acc.converted + (chunks.length - errors.length),
      errors: acc.errors + errors.length,
    }
  }, {
    total_files: 0,
    total_chunks: 0,
    total_questions: 0,
    converted: 0,
    errors: 0,
  })
}

// gives us a chance to replace the raw HTML that is rendered as a "message"
// the key thing this does right now is render links to files that the AI has been
// told to reference in it's answer - because the session metadata keeps a map
// of document_ids to filenames, we can replace the document_id with a link to the
// document in the filestore
export const replaceMessageText = (
  message: string,
  session: ISession,
  getFileURL: (filename: string) => string,
): string => {
  const document_ids = session.config.document_ids || {}
  
  // More detailed debug logging about document IDs
  console.debug(`Session ${session.id} document_ids:`, document_ids)
  console.debug(`Session ${session.id} document_group_id:`, session.config.document_group_id)
  console.debug(`Session ${session.id} parent_app:`, session.parent_app)
  console.debug(`Session ${session.id} session type:`, session.type)
  console.debug(`Session ${session.id} session mode:`, session.mode)
  console.debug(`Session ${session.id} metadata:`, session.config)
  
  // Get all non-text files from the interactions
  const allNonTextFiles = session.interactions.reduce((acc: string[], interaction) => {
    if (!interaction.files || interaction.files.length <= 0) return acc
    return acc.concat(interaction.files.filter(f => f.match(/\.txt$/i) ? false : true))
  }, [])

  // STEP 1: First, check if there are RAG citation blocks and extract them
  // This needs to happen BEFORE any document ID replacement
  const ragCitationRegex = /(?:---\s*)?\s*<excerpts>([\s\S]*?)<\/excerpts>\s*(?:---\s*)?$/;
  const ragMatch = message.match(ragCitationRegex);
  
  // Also check if the LLM directly output citation HTML (happens sometimes)
  const directCitationHtmlRegex = /<div\s+class=["']rag-citations-container["'][\s\S]*?<\/div>\s*<\/div>\s*<\/div>/;
  const directCitationMatch = message.match(directCitationHtmlRegex);
  
  let mainContent = message;
  let citationContent = null;
  
  if (directCitationMatch) {
    // If the LLM has directly output citation HTML, extract it
    console.debug(`Found direct citation HTML in message - extracting for separate processing`);
    citationContent = directCitationMatch[0];
    // Remove citation HTML from main content
    mainContent = message.replace(citationContent, '');
    console.debug(`Extracted citation HTML (${citationContent.length} chars) from main content`);
  } else if (ragMatch) {
    console.debug(`Found RAG citation block in message - extracting for separate processing`);
    citationContent = ragMatch[0];
    // Remove citation block from main content to prevent document ID replacement in citations
    mainContent = message.replace(citationContent, '');
    console.debug(`Extracted citation content (${citationContent.length} chars) from main content`);
  }
  
  // STEP 2: Process document ID replacements on the main content only
  let resultContent = mainContent;
  let documentReferenceCounter = 0;
  
  Object.keys(document_ids).forEach(filename => {
    const document_id = document_ids[filename]
    let searchPattern: RegExp | null = null;
    
    // Use different patterns based on what's in the message
    if (resultContent.indexOf(`[DOC_ID:${document_id}]`) >= 0) {
      // Exact format match: [DOC_ID:ee4fbace49]
      searchPattern = new RegExp(`\\[DOC_ID:${document_id}\\]`, 'g')
      console.debug(`Using exact match pattern for [DOC_ID:${document_id}]`);
    } else if (resultContent.indexOf(`DOC_ID:${document_id}`) >= 0) {
      // Pattern with DOC_ID prefix: For more details, please refer to the original document [DOC_ID:ee4fbace49].
      searchPattern = new RegExp(`\\[.*DOC_ID:${document_id}.*?\\]`, 'g')
      console.debug(`Using DOC_ID prefix pattern for ${document_id}`);
    } else if (resultContent.indexOf(document_id) >= 0) {
      // Raw ID match
      searchPattern = new RegExp(`${document_id}`, 'g')
      console.debug(`Using raw ID pattern for ${document_id}`);
    }
    
    if (!searchPattern) {
      console.debug(`No pattern found for document ID: ${document_id}`);
      return;
    }
    
    documentReferenceCounter++
    
    let link: string;
    // Check if this is an app session
    if (session.parent_app) {
      // For app sessions, create a direct link to the original document
      // Use the original filename without attempting to find it in interactions
      const displayName = filename.split('/').pop() || filename; // Get just the filename part
      console.debug(`Creating app session link for document: ${displayName}, ID: ${document_id}`);
      link = `<a target="_blank" style="color: white;" href="${getFileURL(filename)}">[${documentReferenceCounter}]</a>`;
    } else {
      // Regular session - try to find the file in the interactions
      const baseFilename = filename.replace(/\.txt$/i, '')
      const sourceFilename = allNonTextFiles.find(f => f.indexOf(baseFilename) == 0)
      if (!sourceFilename) {
        console.debug(`Could not find source file for ${filename} with ID ${document_id}`);
        // If we can't find a matching file, still create a link to the original filename
        link = `<a target="_blank" style="color: white;" href="${getFileURL(filename)}">[${documentReferenceCounter}]</a>`;
      } else {
        link = `<a target="_blank" style="color: white;" href="${getFileURL(sourceFilename)}">[${documentReferenceCounter}]</a>`;
      }
    }
    
    resultContent = resultContent.replace(searchPattern, link)
  })

  const document_group_id = session.config.document_group_id
  let groupSearchPattern = ''
  if (resultContent.indexOf(`[DOC_GROUP:${document_group_id}]`) >= 0) {
    groupSearchPattern = `[DOC_GROUP:${document_group_id}]`
  } else if (resultContent.indexOf(document_group_id) >= 0) {
    groupSearchPattern = document_group_id
  }

  if (groupSearchPattern) {
    const link = `<a style="color: white;" href="javascript:_helixHighlightAllFiles()">[group]</a>`
    resultContent = resultContent.replace(groupSearchPattern, link)
  }
  
  // STEP 3: If there were RAG citations, process them separately
  if (citationContent) {
    // Process the citation content - handle both XML citation blocks and direct HTML
    let citationsBody = '';
    let needsProcessing = true;
    
    if (directCitationMatch && directCitationMatch[0] === citationContent) {
      // If it's direct HTML, just use it as is
      resultContent += citationContent;
      needsProcessing = false;
    } else if (ragMatch) {
      // For XML format citations, extract the body for processing
      citationsBody = ragMatch[1]; // We know this exists because ragMatch is not null
    }
    
    // Only process if it's XML format, not direct HTML
    if (needsProcessing) {
      // First, escape any XML tags to prevent HTML parsing issues
      // This is especially important for malformed XML like in the example
      const escapedCitationsBody = citationsBody.replace(
        /<(\/?)(?!(\/)?excerpt|document_id|snippet|\/document_id|\/snippet|\/excerpt)([^>]*)>/g, 
        (match: string, p1: string, p2: string, p3: string) => `&lt;${p1}${p3}&gt;`
      );
      
      console.debug(`Processing citation content with escaped XML tags`);
      
      // Parse the individual excerpts
      const excerptRegex = /<excerpt>([\s\S]*?)<\/excerpt>/g;
      let excerptMatch;
      let excerpts = [];
      
      while ((excerptMatch = excerptRegex.exec(escapedCitationsBody)) !== null) {
        try {
          const excerptContent = excerptMatch[1];
          
          // Use more robust patterns to extract document_id and snippet that can handle malformed content
          let docId = '';
          let snippet = '';
          
          // Extract document_id - handle various possible formats
          const docIdMatch = excerptContent.match(/<document_id>([\s\S]*?)<\/document_id>/);
          if (docIdMatch) {
            docId = docIdMatch[1].trim();
            
            // Special handling for cases where the LLM put an HTML link inside document_id
            if (docId.includes('<a') || docId.includes('href=')) {
              // Try to extract the actual document ID from the link
              // Common patterns might be like href="...id=DOCID" or similar
              const linkDocIdMatch = docId.match(/\/([^\/]+?)\.pdf/) || 
                                  docId.match(/\/([^\/]+?)\.docx/) || 
                                  docId.match(/\/(app_[a-zA-Z0-9]+)/);
              
              if (linkDocIdMatch) {
                docId = linkDocIdMatch[1];
                console.debug(`Extracted document ID ${docId} from malformed link in citation`);
              } else {
                // If we can't extract a proper doc ID, generate a fallback ID
                docId = `doc_${excerpts.length + 1}`;
                console.debug(`Using fallback document ID ${docId} for malformed citation`);
              }
            }
          } else {
            // If no document_id was found, create a fallback
            docId = `doc_${excerpts.length + 1}`;
            console.debug(`No document_id found in excerpt, using fallback: ${docId}`);
          }
          
          // Extract snippet
          const snippetMatch = excerptContent.match(/<snippet>([\s\S]*?)<\/snippet>/);
          if (snippetMatch) {
            snippet = snippetMatch[1].trim();
            
            // Ensure we properly escape any HTML in the snippet
            snippet = snippet
              .replace(/&/g, '&amp;')
              .replace(/</g, '&lt;')
              .replace(/>/g, '&gt;')
              .replace(/"/g, '&quot;')
              .replace(/'/g, '&#039;');
            
            // Handle inline code with backticks
            snippet = snippet.replace(/`([^`]+)`/g, '<code>$1</code>');
          } else {
            snippet = "No content available";
            console.debug(`No snippet found in excerpt`);
          }
          
          // Find a matching document for this ID or URL
          let filename = "";
          let fileUrl = "";
          let fileFound = false;
          
          // First try direct document_id match
          for (const [fname, id] of Object.entries(document_ids)) {
            if (id === docId) {
              filename = fname.split('/').pop() || fname;
              
              // Create the file URL
              if (session.parent_app) {
                fileUrl = getFileURL(fname);
              } else {
                const baseFilename = fname.replace(/\.txt$/i, '');
                const sourceFilename = allNonTextFiles.find(f => f.indexOf(baseFilename) === 0);
                fileUrl = sourceFilename ? getFileURL(sourceFilename) : getFileURL(fname);
              }
              
              fileFound = true;
              break;
            }
          }
          
          // If no direct match was found, try to extract from the document_id if it contains a URL
          if (!fileFound && docId.includes('http')) {
            // Extract the filename from the URL
            const urlFilenameMatch = docId.match(/\/([^\/]+\.[^\/\.]+)($|\?)/);
            if (urlFilenameMatch) {
              filename = urlFilenameMatch[1];
              fileUrl = docId;
              fileFound = true;
            }
          }
          
          // Add this excerpt to our processed array
          excerpts.push({
            docId,
            snippet,
            filename: filename || "Unknown document",
            fileUrl: fileUrl || "#"
          });
        } catch (error) {
          console.error("Error processing excerpt:", error);
          // Continue with the next excerpt
        }
      }
      
      // Convert the excerpts into a citation display
      if (excerpts.length > 0) {
        // Create a citation container div
        let citationHtml = `<div class="rag-citations-container">
          <div class="rag-citations-header">Citations</div>
          <div class="rag-citations-list">`;
        
        // Add each excerpt as a citation item
        excerpts.forEach(excerpt => {
          // Determine icon based on file type
          let iconType = 'document';
          if (excerpt.filename) {
            const ext = excerpt.filename.split('.').pop()?.toLowerCase();
            if (ext === 'pdf') iconType = 'pdf';
            else if (['doc', 'docx'].includes(ext || '')) iconType = 'word';
            else if (['xls', 'xlsx'].includes(ext || '')) iconType = 'excel';
            else if (['ppt', 'pptx'].includes(ext || '')) iconType = 'powerpoint';
            else if (['jpg', 'jpeg', 'png', 'gif'].includes(ext || '')) iconType = 'image';
          }
          
          citationHtml += `
            <div class="rag-citation-item">
              <div class="rag-citation-icon ${iconType}-icon"></div>
              <div class="rag-citation-content">
                <div class="rag-citation-title">
                  <a href="${excerpt.fileUrl}" target="_blank">${excerpt.filename}</a>
                </div>
                <div class="rag-citation-snippet">${excerpt.snippet}</div>
              </div>
            </div>`;
        });
        
        // Close the container
        citationHtml += `
          </div>
        </div>`;
        
        // Add the formatted citations to the result
        resultContent += citationHtml;
      } else {
        // No valid excerpts found, just add the original content
        console.debug(`No valid excerpts found in citation content, using original`);
        resultContent += citationContent;
      }
    }
  }

  return resultContent;
}

// Helper function to escape special characters in a string for use in RegExp
function escapeRegExp(string: string): string {
  return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); // $& means the whole matched string
}

export const getNewSessionBreadcrumbs = ({
  mode,
  type,
  ragEnabled,
  finetuneEnabled,
  app,
}: {
  mode: ISessionMode,
  type: ISessionType,
  ragEnabled: boolean,
  finetuneEnabled: boolean,
  app?: IApp,
}): IPageBreadcrumb[] => {

  if (mode == SESSION_MODE_FINETUNE) {
    let txt = "Add Documents"
    if (type == SESSION_TYPE_IMAGE) {
      txt += " (image style and objects)"
    } else if (ragEnabled && finetuneEnabled) {
      txt += " (hybrid RAG + Fine-tuning)"
    } else if (ragEnabled) {
      txt += " (RAG)"
    } else if (finetuneEnabled) {
      txt += " (Fine-tuning on knowledge)"
    }
    return [{
      title: txt,
    }]
  } else if (app) {
    return [{
      title: 'App Store',
      routeName: 'appstore',
    }, {
      title: getAppName(app),
    }]
  }

  return []
}