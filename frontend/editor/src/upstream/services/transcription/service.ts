import type {TranscriptionLanguage,TranscriptionResult,TranscriptionProgress,TranscriptionModelId} from '@/transcription/types';
/** 6eek policy: inference is server-managed through OpenRouter only.
 * Keep caption-file import and timeline editing available while a paid, metered
 * OpenRouter transcription adapter is not configured. Never download local AI models.
 */
class TranscriptionService {
 async transcribe(_options:{audioData:Float32Array;language?:TranscriptionLanguage;modelId?:TranscriptionModelId;onProgress?:(progress:TranscriptionProgress)=>void}):Promise<TranscriptionResult>{
  throw new Error('Automatic captions are not available yet. Import an SRT or ASS caption file instead.');
 }
 cancel(){} terminate(){}
}
export const transcriptionService=new TranscriptionService();
