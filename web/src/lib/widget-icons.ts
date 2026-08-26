import {Activity,AudioWaveform,Badge,Binary,Blend,Box,ChartArea,ChartBarStacked,ChartColumn,ChartColumnBig,ChartLine,ChartNoAxesColumnIncreasing,ChartPie,CircleDotDashed,CircleGauge,Crosshair,Equal,Gauge,GitBranch,GraduationCap,Grid2X2,Grid3X3,ListChecks,ListOrdered,Orbit,PanelsTopLeft,Radar,Scale,ScatterChart,Siren,Spline,Table2,TableProperties,Terminal,TrendingDown,Waves,type LucideIcon} from "lucide-react";
import type {DashboardWidgetType} from "@/lib/dashboard-widgets";

export const widgetIcons:Record<DashboardWidgetType,LucideIcon>={
  lineplot:ChartLine,loss_curve:TrendingDown,learning_curve:GraduationCap,anomaly_timeline:Siren,
  feature_importance:ListOrdered,shap_summary:Binary,partial_dependence:Spline,embedding_scatter:Orbit,cluster_scatter:Blend,
  barplot:ChartColumn,area_chart:ChartArea,stacked_bar:ChartBarStacked,scatterplot:ScatterChart,starplot:Radar,
  histogram:ChartNoAxesColumnIncreasing,boxplot:Box,violin:AudioWaveform,heatmap:Grid3X3,correlation_heatmap:TableProperties,
  confusion_matrix:Grid2X2,data_grid:Table2,roc_curve:Activity,precision_recall_curve:Crosshair,calibration_curve:Scale,
  prediction_vs_actual:Equal,residual_plot:Waves,bubble_chart:CircleDotDashed,parallel_coordinates:GitBranch,pie_chart:ChartPie,
  donut_chart:CircleGauge,treemap:PanelsTopLeft,waterfall:ChartColumnBig,progress:ListChecks,logs:Terminal,kpi:Badge,gauge:Gauge,
};
