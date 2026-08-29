import { useState, useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { ComboPage } from "./ComboPage";
import { OptimizationPage } from "./OptimizationPage";
import { ExecAnalysisPage } from "./ExecAnalysisPage";
import { EvalPage } from "./EvalPage";
import { useAppStore } from "../stores/appStore";
import * as cmd from "../utils/commands";
import { useTranslation } from "../i18n";

type WorkbenchTab = "combos" | "optimization" | "templates" | "analysis" | "eval";

const TAB_LABEL_KEYS: Record<WorkbenchTab, string> = {
  combos: "workbench.tab.combos",
  optimization: "workbench.tab.optimization",
  templates: "workbench.tab.templates",
  analysis: "workbench.tab.analysis",
  eval: "workbench.tab.eval",
};

type TaskType = "general_chat" | "tool_calling" | "structured_output" | "long_context" | "vision";

const TASK_TYPE_LABEL_KEYS: Record<TaskType, string> = {
  general_chat: "workbench.task.general_chat",
  tool_calling: "workbench.task.tool_calling",
  structured_output: "workbench.task.structured_output",
  long_context: "workbench.task.long_context",
  vision: "workbench.task.vision",
};

const TASK_TYPE_DESC_KEYS: Record<TaskType, string> = {
  general_chat: "workbench.taskDesc.general_chat",
  tool_calling: "workbench.taskDesc.tool_calling",
  structured_output: "workbench.taskDesc.structured_output",
  long_context: "workbench.taskDesc.long_context",
  vision: "workbench.taskDesc.vision",
};

export function OptimizationWorkbenchPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const setTaskProfiles = useAppStore((s) => s.setTaskProfiles);
  const { t } = useTranslation();
  const initialTab = (searchParams.get("tab") as WorkbenchTab) || "combos";
  const [activeTab, setActiveTab] = useState<WorkbenchTab>(
    ["combos", "optimization", "templates", "analysis", "eval"].includes(initialTab) ? initialTab : "combos"
  );
  const [activeTaskType, setActiveTaskType] = useState<TaskType>("general_chat");

  useEffect(() => {
    const tab = searchParams.get("tab") as WorkbenchTab;
    if (tab && ["combos", "optimization", "templates", "analysis", "eval"].includes(tab) && tab !== activeTab) {
      setActiveTab(tab);
    }
  }, [searchParams]);

  useEffect(() => {
    void cmd.getTaskProfiles().then((profiles) => setTaskProfiles(profiles || [])).catch(() => undefined);
  }, [setTaskProfiles]);

  const switchTab = (tab: WorkbenchTab) => {
    setActiveTab(tab);
    setSearchParams({ tab }, { replace: true });
  };

  const switchTaskType = (taskType: TaskType) => {
    setActiveTaskType(taskType);
  };

  return (
    <div className="page workbench-page">
      <div className="page-header">
        <div>
          <h1>{t("workbench.title")}</h1>
          <p className="page-subtitle">
            {t("workbench.subtitle")}
          </p>
        </div>
      </div>

      <div className="tab-bar">
        {(["combos", "optimization", "templates", "analysis", "eval"] as WorkbenchTab[]).map((tab) => (
          <button
            key={tab}
            className={activeTab === tab ? "active" : ""}
            onClick={() => switchTab(tab)}
          >
            {t(TAB_LABEL_KEYS[tab])}
          </button>
        ))}
      </div>

      {activeTab === "optimization" && (
        <div className="task-type-toolbar">
          <div className="task-type-label">{t("workbench.taskTypeLabel")}</div>
          <div className="task-type-buttons">
            {(Object.keys(TASK_TYPE_LABEL_KEYS) as TaskType[]).map((taskType) => (
              <button
                key={taskType}
                className={`task-type-btn ${activeTaskType === taskType ? "active" : ""}`}
                onClick={() => switchTaskType(taskType)}
                title={t(TASK_TYPE_DESC_KEYS[taskType])}
              >
                {t(TASK_TYPE_LABEL_KEYS[taskType])}
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="tab-content">
        {activeTab === "combos" && <ComboPage embedded />}
        {activeTab === "optimization" && <OptimizationPage embedded taskType={activeTaskType} />}
        {activeTab === "templates" && <ComboPage embedded showTemplatesTab />}
        {activeTab === "analysis" && <ExecAnalysisPage />}
        {activeTab === "eval" && <EvalPage />}
      </div>
    </div>
  );
}
