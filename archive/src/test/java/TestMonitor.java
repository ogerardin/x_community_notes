import lukfor.progress.Components;
import lukfor.progress.TaskService;
import lukfor.progress.renderer.IProgressIndicator;

import static lukfor.progress.Components.*;

public class TestMonitor {

    public static void main(String[] args) {

        IProgressIndicator[] components = {
//                Components.PROGRESS_BAR,
//                Components.PROGRESS_BAR_MODERN,
//                Components.PROGRESS_BAR_MINIMAL,
//                SPINNER,
                Components.group(SPINNER, SPACE, TASK_NAME)
//                Components.TIME,
//                Components.ETA,
//                Components.PROGRESS_LABEL,
//                Components.TASK_NAME,
//                Components.DEFAULT,
//                Components.MODERN,
//                Components.MINIMAL,
//                Components.FILE,
        };

        for (IProgressIndicator component : components) {
            System.out.println(component.getClass());
            TaskService.monitor(component).run(monitor -> {
                monitor.begin("Test task...", 100);

                for (int i = 0; i <= 100; i++) {
                    Thread.sleep(25); // Simulate some work
                    monitor.worked(1);
                    monitor.update("Progress: " + i + "%");
                }

                monitor.update("Task completed!");
                monitor.done();
            });
        }

    }
}
