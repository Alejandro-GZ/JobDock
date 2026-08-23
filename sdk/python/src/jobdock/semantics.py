"""Canonical semantic tags understood by JobDock's official dashboards.

Generated from internal/httpapi/catalog/observability.json. Do not edit values
without updating the canonical catalog and its cross-language contract tests.
"""

from enum import Enum


class _SemanticTag(str, Enum):
    def __str__(self) -> str:
        return self.value


class MetricRole(_SemanticTag):
    LOSS="metric:loss"; OBJECTIVE="metric:objective"; LEARNING_RATE="metric:learning_rate"; WEIGHT_DECAY="metric:weight_decay"
    GRADIENT_NORM="metric:gradient_norm"; GRADIENT_SCALE="metric:gradient_scale"; PARAMETER_NORM="metric:parameter_norm"; UPDATE_NORM="metric:update_norm"
    MOMENTUM="metric:momentum"; BETA1="metric:beta1"; BETA2="metric:beta2"; TEMPERATURE="metric:temperature"; EPOCH="metric:epoch"
    STEP_TIME="metric:step_time"; BATCH_TIME="metric:batch_time"; DATA_TIME="metric:data_time"; THROUGHPUT="metric:throughput"
    SAMPLES_PER_SECOND="metric:samples_per_second"; TOKENS_PER_SECOND="metric:tokens_per_second"
    ACCURACY="metric:accuracy"; BALANCED_ACCURACY="metric:balanced_accuracy"; TOP_K_ACCURACY="metric:top_k_accuracy"; PRECISION="metric:precision"
    RECALL="metric:recall"; SENSITIVITY="metric:sensitivity"; SPECIFICITY="metric:specificity"; F1="metric:f1"; F_BETA="metric:f_beta"
    MCC="metric:mcc"; COHEN_KAPPA="metric:cohen_kappa"; JACCARD="metric:jaccard"; HAMMING_LOSS="metric:hamming_loss"
    LOG_LOSS="metric:log_loss"; BRIER_SCORE="metric:brier_score"; ROC_AUC="metric:roc_auc"; PR_AUC="metric:pr_auc"
    AVERAGE_PRECISION="metric:average_precision"; FALSE_POSITIVE_RATE="metric:false_positive_rate"; FALSE_NEGATIVE_RATE="metric:false_negative_rate"
    TRUE_POSITIVE_RATE="metric:true_positive_rate"; TRUE_NEGATIVE_RATE="metric:true_negative_rate"; ECE="metric:ece"; MCE="metric:mce"; NLL="metric:nll"
    MAE="metric:mae"; MSE="metric:mse"; RMSE="metric:rmse"; R2="metric:r2"; MAPE="metric:mape"; SMAPE="metric:smape"; WAPE="metric:wape"
    MASE="metric:mase"; MSLE="metric:msle"; RMSLE="metric:rmsle"; MEDIAN_ABSOLUTE_ERROR="metric:median_absolute_error"; MAX_ERROR="metric:max_error"
    EXPLAINED_VARIANCE="metric:explained_variance"; MEAN_BIAS_ERROR="metric:mean_bias_error"; PINBALL_LOSS="metric:pinball_loss"
    QUANTILE_LOSS="metric:quantile_loss"; CRPS="metric:crps"; COVERAGE="metric:coverage"; INTERVAL_WIDTH="metric:interval_width"
    DIRECTIONAL_ACCURACY="metric:directional_accuracy"
    SILHOUETTE_SCORE="metric:silhouette_score"; DAVIES_BOULDIN="metric:davies_bouldin"; CALINSKI_HARABASZ="metric:calinski_harabasz"
    ADJUSTED_RAND_INDEX="metric:adjusted_rand_index"; NORMALIZED_MUTUAL_INFO="metric:normalized_mutual_info"; ADJUSTED_MUTUAL_INFO="metric:adjusted_mutual_info"
    HOMOGENEITY="metric:homogeneity"; COMPLETENESS="metric:completeness"; V_MEASURE="metric:v_measure"; FOWLKES_MALLOWS="metric:fowlkes_mallows"
    INERTIA="metric:inertia"; RECONSTRUCTION_ERROR="metric:reconstruction_error"; EXPLAINED_VARIANCE_RATIO="metric:explained_variance_ratio"
    TRUSTWORTHINESS="metric:trustworthiness"; ALIGNMENT="metric:alignment"; UNIFORMITY="metric:uniformity"
    IOU="metric:iou"; MEAN_IOU="metric:mean_iou"; DICE="metric:dice"; MEAN_AVERAGE_PRECISION="metric:mean_average_precision"; MAP_50="metric:map_50"
    MAP_75="metric:map_75"; AVERAGE_RECALL="metric:average_recall"; PIXEL_ACCURACY="metric:pixel_accuracy"; PANOPTIC_QUALITY="metric:panoptic_quality"
    SEGMENTATION_QUALITY="metric:segmentation_quality"; RECOGNITION_QUALITY="metric:recognition_quality"; KEYPOINT_AP="metric:keypoint_ap"
    KEYPOINT_AR="metric:keypoint_ar"; OKS="metric:oks"; PSNR="metric:psnr"; SSIM="metric:ssim"; LPIPS="metric:lpips"; FID="metric:fid"; KID="metric:kid"
    CLIP_SCORE="metric:clip_score"
    PERPLEXITY="metric:perplexity"; BLEU="metric:bleu"; SACREBLEU="metric:sacrebleu"; ROUGE_1="metric:rouge_1"; ROUGE_2="metric:rouge_2"
    ROUGE_L="metric:rouge_l"; METEOR="metric:meteor"; CHRF="metric:chrf"; TER="metric:ter"; WER="metric:wer"; CER="metric:cer"
    EXACT_MATCH="metric:exact_match"; TOKEN_ACCURACY="metric:token_accuracy"; SEQUENCE_ACCURACY="metric:sequence_accuracy"
    BERT_SCORE="metric:bert_score"; COMET="metric:comet"; SPEECH_QUALITY_MOS="metric:speech_quality_mos"
    INCEPTION_SCORE="metric:inception_score"; DIVERSITY_SCORE="metric:diversity_score"; NOVELTY_SCORE="metric:novelty_score"; TOXICITY="metric:toxicity"
    BIAS_SCORE="metric:bias_score"; HALLUCINATION_RATE="metric:hallucination_rate"; FACTUALITY="metric:factuality"; FAITHFULNESS="metric:faithfulness"
    ANSWER_RELEVANCE="metric:answer_relevance"; CONTEXT_PRECISION="metric:context_precision"; CONTEXT_RECALL="metric:context_recall"
    GROUNDEDNESS="metric:groundedness"; WIN_RATE="metric:win_rate"; REWARD="metric:reward"; KL_DIVERGENCE="metric:kl_divergence"
    ENTROPY="metric:entropy"; CROSS_ENTROPY="metric:cross_entropy"; COSINE_SIMILARITY="metric:cosine_similarity"; RECALL_AT_K="metric:recall_at_k"
    PRECISION_AT_K="metric:precision_at_k"; MRR="metric:mrr"; NDCG="metric:ndcg"; HIT_RATE="metric:hit_rate"; MAP_AT_K="metric:map_at_k"
    EPISODE_REWARD="metric:episode_reward"; EPISODE_LENGTH="metric:episode_length"; SUCCESS_RATE="metric:success_rate"; POLICY_LOSS="metric:policy_loss"
    VALUE_LOSS="metric:value_loss"; Q_LOSS="metric:q_loss"; ACTOR_LOSS="metric:actor_loss"; CRITIC_LOSS="metric:critic_loss"
    ENTROPY_BONUS="metric:entropy_bonus"; APPROX_KL="metric:approx_kl"; CLIP_FRACTION="metric:clip_fraction"
    EXPLAINED_VALUE_VARIANCE="metric:explained_value_variance"; ADVANTAGE_MEAN="metric:advantage_mean"; RETURN_MEAN="metric:return_mean"
    LATENCY="metric:latency"; P50_LATENCY="metric:p50_latency"; P90_LATENCY="metric:p90_latency"; P95_LATENCY="metric:p95_latency"
    P99_LATENCY="metric:p99_latency"; REQUEST_RATE="metric:request_rate"; ERROR_RATE="metric:error_rate"; TOKEN_THROUGHPUT="metric:token_throughput"
    TIME_TO_FIRST_TOKEN="metric:time_to_first_token"; TIME_PER_OUTPUT_TOKEN="metric:time_per_output_token"
    GPU_UTILIZATION="metric:gpu_utilization"; MEMORY_UTILIZATION="metric:memory_utilization"
    TRIAL_SCORE="metric:trial_score"; BEST_SCORE="metric:best_score"; RANK="metric:rank"; PARAMETER_IMPORTANCE="metric:parameter_importance"
    FOLD_SCORE="metric:fold_score"; MEAN_SCORE="metric:mean_score"; STD_SCORE="metric:std_score"
    CONFIDENCE_INTERVAL_WIDTH="metric:confidence_interval_width"; PARETO_RANK="metric:pareto_rank"; HYPERVOLUME="metric:hypervolume"
    ANOMALY_SCORE="metric:anomaly_score"; ANOMALY_PRECISION="metric:anomaly_precision"; ANOMALY_RECALL="metric:anomaly_recall"
    ANOMALY_F1="metric:anomaly_f1"; FALSE_ALARM_RATE="metric:false_alarm_rate"; DETECTION_DELAY="metric:detection_delay"


class Phase(_SemanticTag):
    DATA_COLLECTION="phase:data_collection"; DATA_VALIDATION="phase:data_validation"; PREPROCESSING="phase:preprocessing"; AUGMENTATION="phase:augmentation"
    WARMUP="phase:warmup"; PRETRAINING="phase:pretraining"; TRAIN="phase:train"; FINE_TUNING="phase:fine_tuning"; VALIDATION="phase:validation"
    TEST="phase:test"; CALIBRATION="phase:calibration"; EVALUATION="phase:evaluation"; INFERENCE="phase:inference"; PROFILING="phase:profiling"
    BENCHMARK="phase:benchmark"; DISTILLATION="phase:distillation"; PRUNING="phase:pruning"; QUANTIZATION="phase:quantization"; EXPORT="phase:export"
    HPO_SEARCH="phase:hpo_search"; HPO_TRIAL="phase:hpo_trial"; MODEL_SELECTION="phase:model_selection"; CROSS_VALIDATION="phase:cross_validation"
    ENSEMBLE="phase:ensemble"; ABLATION="phase:ablation"; HOLDOUT="phase:holdout"; DEPLOYMENT="phase:deployment"; MONITORING="phase:monitoring"
    CANARY="phase:canary"; POSTPROCESSING="phase:postprocessing"


SEMANTIC_CATALOG_VERSION = 1
