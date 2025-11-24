/* eslint-disable */
/* tslint:disable */
// @ts-nocheck
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

import {
  OpenaiChatCompletionRequest,
  OpenaiChatCompletionResponse,
  TypesFlexibleEmbeddingRequest,
  TypesFlexibleEmbeddingResponse,
  TypesOpenAIModelsList,
} from "./data-contracts";
import { ContentType, HttpClient, RequestParams } from "./http-client";

export class V1<
  SecurityDataType = unknown,
> extends HttpClient<SecurityDataType> {
  /**
   * @description Creates a model response for the given chat conversation.
   *
   * @tags chat
   * @name ChatCompletionsCreate
   * @summary Stream responses for chat
   * @request POST:/v1/chat/completions
   * @secure
   */
  chatCompletionsCreate = (
    request: OpenaiChatCompletionRequest,
    params: RequestParams = {},
  ) =>
    this.request<OpenaiChatCompletionResponse, any>({
      path: `/v1/chat/completions`,
      method: "POST",
      body: request,
      secure: true,
      type: ContentType.Json,
      ...params,
    });
  /**
   * @description Creates an embedding vector representing the input text. Supports both standard OpenAI embedding format and Chat Embeddings API format with messages.
   *
   * @tags embeddings
   * @name EmbeddingsCreate
   * @summary Creates an embedding vector representing the input text
   * @request POST:/v1/embeddings
   * @secure
   */
  embeddingsCreate = (
    request: TypesFlexibleEmbeddingRequest,
    params: RequestParams = {},
  ) =>
    this.request<TypesFlexibleEmbeddingResponse, any>({
      path: `/v1/embeddings`,
      method: "POST",
      body: request,
      secure: true,
      type: ContentType.Json,
      ...params,
    });
  /**
   * No description
   *
   * @name ModelsList
   * @request GET:/v1/models
   * @secure
   */
  modelsList = (
    query?: {
      /** Provider */
      provider?: string;
    },
    params: RequestParams = {},
  ) =>
    this.request<TypesOpenAIModelsList[], any>({
      path: `/v1/models`,
      method: "GET",
      query: query,
      secure: true,
      ...params,
    });
}
